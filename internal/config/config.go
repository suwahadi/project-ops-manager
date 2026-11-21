package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const EncryptionKeySize = 32

type Config struct {
	BaseURL          string
	DatabaseURL      string
	EncryptionKey    []byte
	EncryptionKeyNew []byte
	AdminEmail       string
	AdminPassword    string
	ChromiumURL      string
	UploadsDir       string
	ListenAddr       string
}

type Getenv func(string) string

func Load(getenv Getenv) (Config, error) {
	cfg := Config{
		BaseURL:       strings.TrimRight(strings.TrimSpace(getenv("OPS_BASE_URL")), "/"),
		DatabaseURL:   strings.TrimSpace(getenv("OPS_DATABASE_URL")),
		AdminEmail:    strings.TrimSpace(getenv("OPS_ADMIN_EMAIL")),
		AdminPassword: getenv("OPS_ADMIN_PASSWORD"),
		ChromiumURL:   strings.TrimSpace(getenv("OPS_CHROMIUM_URL")),
		UploadsDir:    strings.TrimSpace(getenv("OPS_UPLOADS_DIR")),
		ListenAddr:    strings.TrimSpace(getenv("OPS_LISTEN_ADDR")),
	}
	if cfg.UploadsDir == "" {
		cfg.UploadsDir = "/data/uploads"
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	var errs []error
	if cfg.BaseURL == "" {
		errs = append(errs, errors.New("OPS_BASE_URL is required"))
	} else if u, err := url.Parse(cfg.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, errors.New("OPS_BASE_URL must be an absolute http or https URL"))
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("OPS_DATABASE_URL is required"))
	}
	if cfg.ChromiumURL == "" {
		errs = append(errs, errors.New("OPS_CHROMIUM_URL is required"))
	}
	key, err := ParseEncryptionKey(getenv("OPS_ENCRYPTION_KEY"))
	if err != nil {
		errs = append(errs, fmt.Errorf("OPS_ENCRYPTION_KEY: %w", err))
	}
	cfg.EncryptionKey = key
	if raw := strings.TrimSpace(getenv("OPS_ENCRYPTION_KEY_NEW")); raw != "" {
		newKey, err := ParseEncryptionKey(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("OPS_ENCRYPTION_KEY_NEW: %w", err))
		}
		cfg.EncryptionKeyNew = newKey
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ParseEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		return nil, errors.New("must be valid base64")
	}
	if len(key) != EncryptionKeySize {
		return nil, fmt.Errorf("must decode to %d bytes, got %d", EncryptionKeySize, len(key))
	}
	return key, nil
}
