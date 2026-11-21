package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/auth"
	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/config"
	"github.com/suwahadi/project-ops-manager/internal/credential"
	"github.com/suwahadi/project-ops-manager/internal/db"
	"github.com/suwahadi/project-ops-manager/internal/jobs"
	"github.com/suwahadi/project-ops-manager/internal/markdown"
	"github.com/suwahadi/project-ops-manager/internal/provider"
	"github.com/suwahadi/project-ops-manager/internal/settings"
	"github.com/suwahadi/project-ops-manager/internal/web"
	"github.com/suwahadi/project-ops-manager/internal/web/handlers"
	"github.com/suwahadi/project-ops-manager/internal/web/templates"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(ctx, logger)
	case "rotate-key":
		err = rotateKey(ctx, logger)
	case "healthcheck":
		err = healthcheck()
	default:
		err = fmt.Errorf("unknown command %q, expected serve, rotate-key or healthcheck", cmd)
	}
	if err != nil {
		logger.Error("fatal", "command", cmd, "error", err)
		os.Exit(1)
	}
}

func serve(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, logger); err != nil {
		return err
	}
	box, err := settings.OpenBox(ctx, pool, cfg.EncryptionKey)
	if err != nil {
		return err
	}
	now := time.Now
	authSvc := auth.NewService(pool, now)
	created, err := authSvc.EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		return err
	}
	if created {
		logger.Info("admin account created")
	}
	store := settings.NewStore(pool, box)
	renderer, err := templates.New(nil)
	if err != nil {
		return err
	}
	app := handlers.New(handlers.Deps{
		Logger:      logger,
		Pool:        pool,
		Render:      renderer,
		Auth:        authSvc,
		Settings:    store,
		Assets:      asset.NewService(pool, cfg.UploadsDir),
		AssetTypes:  assettype.NewService(pool),
		Clients:     client.NewService(pool),
		Providers:   provider.NewService(pool),
		Credentials: credential.NewService(pool, box),
		Markdown:    markdown.New(),
		Now:         now,
		BaseURL:     cfg.BaseURL,
		Secure:      strings.HasPrefix(cfg.BaseURL, "https://"),
	})

	runner := jobs.NewRunner(pool, logger,
		jobs.Job{
			Name: "session_cleanup", LockKey: db.LockCleanup, Schedule: jobs.Every(24 * time.Hour), RunAtStart: true,
			Run: func(ctx context.Context) error {
				s, a, err := authSvc.Cleanup(ctx)
				if err == nil {
					logger.Info("session cleanup finished", "sessions_deleted", s, "login_attempts_deleted", a)
				}
				return err
			},
		},
	)
	runner.Start(ctx)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           web.NewHandler(app),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.ListenAddr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	runner.Wait()
	logger.Info("shutdown complete")
	return err
}

func rotateKey(ctx context.Context, logger *slog.Logger) error {
	dbURL := os.Getenv("OPS_DATABASE_URL")
	if dbURL == "" {
		return errors.New("OPS_DATABASE_URL is required")
	}
	oldKey, err := config.ParseEncryptionKey(os.Getenv("OPS_ENCRYPTION_KEY"))
	if err != nil {
		return fmt.Errorf("OPS_ENCRYPTION_KEY: %w", err)
	}
	newKey, err := config.ParseEncryptionKey(os.Getenv("OPS_ENCRYPTION_KEY_NEW"))
	if err != nil {
		return fmt.Errorf("OPS_ENCRYPTION_KEY_NEW: %w", err)
	}
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, logger); err != nil {
		return err
	}
	res, err := settings.RotateKey(ctx, pool, oldKey, newKey)
	if err != nil {
		return err
	}
	logger.Info("encryption key rotated", "secrets", res.Secrets, "credentials", res.Credentials, "key_version", res.NewVersion)
	fmt.Fprintln(os.Stderr, "Key rotated. Set OPS_ENCRYPTION_KEY to the new key and remove OPS_ENCRYPTION_KEY_NEW before restarting.")
	return nil
}

func healthcheck() error {
	addr := os.Getenv("OPS_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}
