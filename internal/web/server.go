package web

import (
	"net/http"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/web/handlers"
	"github.com/suwahadi/project-ops-manager/internal/web/middleware"
	"github.com/suwahadi/project-ops-manager/internal/web/static"
)

const maxRequestBody = 256 << 20

func NewHandler(app *handlers.App) http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /login", app.LoginForm)
	public.HandleFunc("POST /login", app.Login)
	public.HandleFunc("GET /healthz", app.Healthz)
	public.HandleFunc("GET /robots.txt", app.Robots)
	public.Handle("GET /static/", staticHandler())

	private := http.NewServeMux()
	registerPrivate(private, app)
	public.Handle("/", middleware.RequireAuth(middleware.CSRF(private)))

	var h http.Handler = public
	h = limitBody(h)
	h = middleware.LoadSession(app.Auth, app.Logger)(h)
	h = middleware.Theme(h)
	h = middleware.SecurityHeaders(app.Secure)(h)
	h = middleware.Logging(app.Logger)(h)
	return h
}

func registerPrivate(mux *http.ServeMux, app *handlers.App) {
	mux.HandleFunc("GET /{$}", app.Dashboard)
	mux.HandleFunc("POST /logout", app.Logout)
	mux.HandleFunc("GET /settings", app.SettingsAccount)
	mux.HandleFunc("POST /settings/account/email", app.SettingsAccountEmail)
	mux.HandleFunc("POST /settings/account/password", app.SettingsAccountPassword)
	mux.HandleFunc("GET /settings/general", app.SettingsGeneral)
	mux.HandleFunc("POST /settings/general", app.SettingsGeneralSave)
	mux.HandleFunc("/", app.NotFound)
}

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
		next.ServeHTTP(w, r)
	})
}

func staticHandler() http.Handler {
	files := http.StripPrefix("/static/", http.FileServer(http.FS(static.FS)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}
