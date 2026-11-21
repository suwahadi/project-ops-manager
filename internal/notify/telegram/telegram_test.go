package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/suwahadi/project-ops-manager/internal/notify"
)

func TestSend(t *testing.T) {
	var got map[string]any
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	s := &Sender{APIBase: srv.URL, Client: srv.Client()}
	err := s.Send(context.Background(), Config{Token: "123:abc", ChatID: "42"}, notify.Message{Subject: "Hi", Body: "There"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/bot123:abc/sendMessage" || got["chat_id"] != "42" || got["text"] != "Hi\n\nThere" {
		t.Fatalf("path %q payload %v", path, got)
	}
}

func TestSendErrorsNeverLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Unauthorized for 999:secret-token"}`))
	}))
	s := &Sender{APIBase: srv.URL, Client: srv.Client()}
	err := s.Send(context.Background(), Config{Token: "999:secret-token", ChatID: "1"}, notify.Message{})
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("expected redacted error, got %v", err)
	}
	srv.Close()
	err = s.Send(context.Background(), Config{Token: "999:secret-token", ChatID: "1"}, notify.Message{})
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("network error leaks token: %v", err)
	}
}
