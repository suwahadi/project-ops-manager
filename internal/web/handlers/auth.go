package handlers

import (
	"errors"
	"net/http"

	"github.com/suwahadi/project-ops-manager/internal/auth"
	"github.com/suwahadi/project-ops-manager/internal/web/middleware"
)

type loginData struct {
	Email string
	Error string
}

func (a *App) LoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.SessionFrom(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.renderLogin(w, r, http.StatusOK, loginData{})
}

func validLoginToken(v string) bool {
	if len(v) != 43 {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (a *App) loginToken(r *http.Request) (string, error) {
	if c, err := r.Cookie(middleware.LoginCSRFCookie); err == nil && validLoginToken(c.Value) {
		return c.Value, nil
	}
	return auth.RandomToken()
}

func (a *App) renderLogin(w http.ResponseWriter, r *http.Request, status int, data loginData) {
	token, err := a.loginToken(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: middleware.LoginCSRFCookie, Value: token, Path: "/login", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
	p := a.page(w, r, "Login", "", data)
	p.CSRF = token
	if err := a.Render.Bare(w, status, "login", p); err != nil {
		a.Logger.Error("render login", "error", err)
	}
}

func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(middleware.LoginCSRFCookie)
	if err != nil || c.Value == "" || c.Value != r.PostFormValue(middleware.CSRFFormField) {
		a.renderLogin(w, r, http.StatusForbidden, loginData{Error: "Your session expired. Please try again."})
		return
	}
	email := r.PostFormValue("email")
	token, err := a.Auth.Login(r.Context(), middleware.ClientIP(r), email, r.PostFormValue("password"))
	switch {
	case errors.Is(err, auth.ErrLockedOut):
		a.renderLogin(w, r, http.StatusTooManyRequests, loginData{Email: email, Error: "Too many failed attempts. Try again in 15 minutes."})
		return
	case errors.Is(err, auth.ErrInvalidCredentials):
		a.renderLogin(w, r, http.StatusUnauthorized, loginData{Email: email, Error: "Invalid email or password."})
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: middleware.LoginCSRFCookie, Value: "", Path: "/login", MaxAge: -1, HttpOnly: true, Secure: a.Secure})
	http.SetCookie(w, &http.Cookie{
		Name: middleware.SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.Secure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(auth.SessionIdleTimeout.Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(middleware.SessionCookie); err == nil {
		if err := a.Auth.Logout(r.Context(), c.Value); err != nil {
			a.Logger.Error("logout", "error", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: middleware.SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *App) Favicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/img/favicon.svg", http.StatusMovedPermanently)
}

func (a *App) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := a.Pool.Ping(r.Context()); err != nil {
		a.Logger.Error("health check database ping", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("database unavailable"))
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (a *App) Robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("User-agent: *\nDisallow: /\n"))
}
