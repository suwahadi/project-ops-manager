package email

import (
	"bytes"
	"strings"
	"testing"

	"github.com/suwahadi/project-ops-manager/internal/notify"
)

func TestBuildMessage(t *testing.T) {
	cfg := Config{Host: "smtp.example.com", Port: 587, From: "ops@example.com", To: "me@example.com"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	msg, err := BuildMessage(cfg, notify.Message{Subject: "Renewal in 7 days: example.com (Domain)", Body: "Body text"})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := msg.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Subject: Renewal in 7 days: example.com (Domain)", "To: <me@example.com>", "Body text"} {
		if !strings.Contains(out, want) {
			t.Errorf("message missing %q:\n%s", want, out)
		}
	}
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
