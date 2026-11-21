package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/web/middleware"
	"github.com/suwahadi/project-ops-manager/internal/web/templates"
)

func loginCookie(t *testing.T, res *http.Response) string {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == middleware.LoginCSRFCookie {
			return c.Value
		}
	}
	t.Fatal("login cookie not set")
	return ""
}

func TestLoginFormReusesTokenAcrossPageLoads(t *testing.T) {
	r, err := templates.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Render: r, Now: time.Now, Secure: true})

	first := httptest.NewRecorder()
	app.LoginForm(first, httptest.NewRequest(http.MethodGet, "/login", nil))
	token := loginCookie(t, first.Result())
	if !strings.Contains(first.Body.String(), `value="`+token+`"`) {
		t.Fatal("form does not carry the cookie token")
	}

	again := httptest.NewRequest(http.MethodGet, "/login", nil)
	again.AddCookie(&http.Cookie{Name: middleware.LoginCSRFCookie, Value: token})
	second := httptest.NewRecorder()
	app.LoginForm(second, again)
	if got := loginCookie(t, second.Result()); got != token {
		t.Fatalf("a second page load must keep the token, got a new one")
	}
	if !strings.Contains(second.Body.String(), `value="`+token+`"`) {
		t.Fatal("second form does not carry the same token")
	}

	forged := httptest.NewRequest(http.MethodGet, "/login", nil)
	forged.AddCookie(&http.Cookie{Name: middleware.LoginCSRFCookie, Value: "short"})
	third := httptest.NewRecorder()
	app.LoginForm(third, forged)
	if got := loginCookie(t, third.Result()); got == "short" || !validLoginToken(got) {
		t.Fatalf("an invalid cookie must be replaced, got %q", got)
	}

	fav := httptest.NewRecorder()
	app.Favicon(fav, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if fav.Code != http.StatusMovedPermanently || fav.Header().Get("Location") != "/static/img/favicon.svg" {
		t.Fatalf("favicon = %d %s", fav.Code, fav.Header().Get("Location"))
	}
}
