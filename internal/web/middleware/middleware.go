package middleware

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/auth"
)

const (
	SessionCookie    = "ops_session"
	LoginCSRFCookie  = "ops_login_csrf"
	ThemeCookie      = "ops_theme"
	CSRFHeader       = "X-CSRF-Token"
	CSRFFormField    = "csrf_token"
	contentSecPolicy = "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
)

type ctxKey int

const (
	sessionKey ctxKey = iota
	themeKey
)

func SecurityHeaders(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", contentSecPolicy)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Frame-Options", "DENY")
			h.Set("X-Robots-Tag", "noindex, nofollow")
			if secure {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				if v := recover(); v != nil {
					logger.Error("panic in handler", "error", v, "stack", string(debug.Stack()), "path", r.URL.Path)
					http.Error(rec, "Internal server error", http.StatusInternalServerError)
				}
				if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/") {
					return
				}
				logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func Theme(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		theme := "system"
		if c, err := r.Cookie(ThemeCookie); err == nil && (c.Value == "light" || c.Value == "dark") {
			theme = c.Value
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), themeKey, theme)))
	})
}

func ThemeFrom(ctx context.Context) string {
	if t, ok := ctx.Value(themeKey).(string); ok {
		return t
	}
	return "system"
}

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (auth.Session, error)
}

func LoadSession(a Authenticator, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(SessionCookie)
			if err == nil && c.Value != "" {
				sess, err := a.Authenticate(r.Context(), c.Value)
				if err == nil {
					r = r.WithContext(context.WithValue(r.Context(), sessionKey, sess))
				} else if err != auth.ErrNoSession {
					logger.Error("authenticate session", "error", err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func SessionFrom(ctx context.Context) (auth.Session, bool) {
	s, ok := ctx.Value(sessionKey).(auth.Session)
	return s, ok
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := SessionFrom(r.Context()); !ok {
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := SessionFrom(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get(CSRFHeader)
		if token == "" {
			token = formValue(r, CSRFFormField)
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(sess.CSRFToken)) != 1 {
			http.Error(w, "Invalid or missing CSRF token. Reload the page and try again.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func formValue(r *http.Request, key string) string {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return ""
		}
		return r.FormValue(key)
	}
	return r.PostFormValue(key)
}

func ClientIP(r *http.Request) netip.Addr {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if addr, err := netip.ParseAddr(strings.TrimSpace(parts[len(parts)-1])); err == nil {
			return addr.Unmap()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap()
	}
	return netip.IPv4Unspecified()
}
