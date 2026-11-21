package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func env(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

func validEnv() map[string]string {
	return map[string]string{
		"OPS_BASE_URL":       "https://ops.example.com/",
		"OPS_DATABASE_URL":   "postgres://u:p@db/ops",
		"OPS_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"OPS_CHROMIUM_URL":   "ws://chromium:9222",
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(validEnv()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseURL != "https://ops.example.com" {
		t.Errorf("base url = %q", cfg.BaseURL)
	}
	if cfg.UploadsDir != "/data/uploads" {
		t.Errorf("uploads dir = %q", cfg.UploadsDir)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("key length = %d", len(cfg.EncryptionKey))
	}
}

func TestLoadRejectsBadEncryptionKey(t *testing.T) {
	cases := map[string]string{
		"missing":    "",
		"not base64": "%%%",
		"too short":  base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"too long":   base64.StdEncoding.EncodeToString(make([]byte, 33)),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			e := validEnv()
			e["OPS_ENCRYPTION_KEY"] = key
			_, err := Load(env(e))
			if err == nil || !strings.Contains(err.Error(), "OPS_ENCRYPTION_KEY") {
				t.Fatalf("expected encryption key error, got %v", err)
			}
		})
	}
}

func TestLoadRequiresBaseURL(t *testing.T) {
	e := validEnv()
	delete(e, "OPS_BASE_URL")
	if _, err := Load(env(e)); err == nil {
		t.Fatal("expected error")
	}
}
