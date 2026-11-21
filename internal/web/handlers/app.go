package handlers

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/auth"
	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/credential"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/document"
	"github.com/suwahadi/project-ops-manager/internal/finance"
	"github.com/suwahadi/project-ops-manager/internal/markdown"
	"github.com/suwahadi/project-ops-manager/internal/provider"
	"github.com/suwahadi/project-ops-manager/internal/rdap"
	"github.com/suwahadi/project-ops-manager/internal/reminder"
	"github.com/suwahadi/project-ops-manager/internal/settings"
	"github.com/suwahadi/project-ops-manager/internal/web/middleware"
	"github.com/suwahadi/project-ops-manager/internal/web/templates"
)

const flashCookie = "ops_flash"

type Deps struct {
	Logger      *slog.Logger
	Pool        *pgxpool.Pool
	Render      *templates.Renderer
	Auth        *auth.Service
	Settings    *settings.Store
	Assets      *asset.Service
	AssetTypes  *assettype.Service
	Clients     *client.Service
	Providers   *provider.Service
	Credentials *credential.Service
	Markdown    *markdown.Renderer
	Queries     *sqlc.Queries
	Documents   *document.Store
	Finance     *finance.Service
	RDAP        *rdap.Service
	Telegram    reminder.TelegramSender
	Email       reminder.EmailSender
	Now         func() time.Time
	BaseURL     string
	Secure      bool
}

type App struct {
	Deps
}

func New(d Deps) *App {
	return &App{Deps: d}
}

type Flash struct {
	Message string
	Error   bool
}

type Page struct {
	Title       string
	Nav         string
	CSRF        string
	Theme       string
	Flash       *Flash
	SearchQuery string
	Data        any
}

func (a *App) page(w http.ResponseWriter, r *http.Request, title, nav string, data any) Page {
	p := Page{Title: title, Nav: nav, Theme: middleware.ThemeFrom(r.Context()), Data: data}
	if sess, ok := middleware.SessionFrom(r.Context()); ok {
		p.CSRF = sess.CSRFToken
	}
	p.Flash = a.takeFlash(w, r)
	return p
}

func (a *App) render(w http.ResponseWriter, r *http.Request, status int, tmpl, title, nav string, data any) {
	if err := a.Render.Page(w, status, tmpl, a.page(w, r, title, nav, data)); err != nil {
		a.Logger.Error("render page", "template", tmpl, "error", err)
	}
}

func (a *App) fragment(w http.ResponseWriter, r *http.Request, status int, tmpl, name string, data any) {
	p := Page{Theme: middleware.ThemeFrom(r.Context()), Data: data}
	if sess, ok := middleware.SessionFrom(r.Context()); ok {
		p.CSRF = sess.CSRFToken
	}
	if err := a.Render.Fragment(w, status, tmpl, name, p); err != nil {
		a.Logger.Error("render fragment", "template", tmpl, "name", name, "error", err)
	}
}

func (a *App) serverError(w http.ResponseWriter, r *http.Request, err error) {
	a.Logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	if isHTMX(r) {
		toastHeader(w, "Something went wrong. Please try again.", true)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
}

func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusNotFound, "notfound", "Not found", "", nil)
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func toastHeader(w http.ResponseWriter, msg string, isErr bool) {
	kind := "success"
	if isErr {
		kind = "error"
	}
	payload := `{"showToast":{"message":` + jsonString(msg) + `,"kind":"` + kind + `"}}`
	w.Header().Set("HX-Trigger", payload)
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r < 0x20 || r == '<' || r == '>' || r == '&':
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[r>>4])
			b.WriteByte("0123456789abcdef"[r&0xf])
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (a *App) setFlash(w http.ResponseWriter, msg string, isErr bool) {
	prefix := "s:"
	if isErr {
		prefix = "e:"
	}
	http.SetCookie(w, &http.Cookie{
		Name: flashCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(prefix + msg)),
		Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode, MaxAge: 60,
	})
}

func (a *App) takeFlash(w http.ResponseWriter, r *http.Request) *Flash {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode})
	raw, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil || len(raw) < 2 {
		return nil
	}
	return &Flash{Message: string(raw[2:]), Error: raw[0] == 'e'}
}

func (a *App) redirectWithFlash(w http.ResponseWriter, r *http.Request, to, msg string, isErr bool) {
	if msg != "" {
		a.setFlash(w, msg, isErr)
	}
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func safeReturnPath(raw, fallback string) string {
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(raw, "//") {
		return fallback
	}
	return u.RequestURI()
}

var errNotFound = errors.New("not found")

type FieldErrors map[string]string

func (f FieldErrors) Add(field, msg string) {
	if _, ok := f[field]; !ok {
		f[field] = msg
	}
}

func (f FieldErrors) Any() bool { return len(f) > 0 }

func (a *App) location(r *http.Request) *time.Location {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.Logger.Error("load settings", "error", err)
		return time.UTC
	}
	return all.General.Location()
}

func (a *App) today(r *http.Request) time.Time {
	return asset.Today(a.Now(), a.location(r))
}
