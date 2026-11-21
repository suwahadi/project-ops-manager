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
	public.HandleFunc("GET /favicon.ico", app.Favicon)
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
	mux.HandleFunc("GET /settings/notifications", app.SettingsNotifications)
	mux.HandleFunc("POST /settings/notifications/telegram", app.SettingsTelegramSave)
	mux.HandleFunc("POST /settings/notifications/email", app.SettingsEmailSave)
	mux.HandleFunc("POST /settings/notifications/telegram/test", app.SettingsTelegramTest)
	mux.HandleFunc("POST /settings/notifications/email/test", app.SettingsEmailTest)

	mux.HandleFunc("GET /search", app.Search)
	mux.HandleFunc("POST /markdown/preview", app.MarkdownPreview)

	mux.HandleFunc("GET /assets", app.AssetList)
	mux.HandleFunc("GET /assets/new", app.AssetNew)
	mux.HandleFunc("GET /assets/fields", app.AssetFields)
	mux.HandleFunc("POST /assets", app.AssetCreate)
	mux.HandleFunc("GET /assets/{id}", app.AssetShow)
	mux.HandleFunc("GET /assets/{id}/edit", app.AssetEdit)
	mux.HandleFunc("POST /assets/{id}", app.AssetUpdate)
	mux.HandleFunc("POST /assets/{id}/cancel", app.AssetCancel)
	mux.HandleFunc("POST /assets/{id}/restore", app.AssetRestore)
	mux.HandleFunc("POST /assets/{id}/delete", app.AssetDelete)
	mux.HandleFunc("POST /assets/{id}/notes", app.AssetNotes)
	mux.HandleFunc("POST /assets/{id}/renew", app.AssetRenew)
	mux.HandleFunc("POST /assets/{id}/rdap", app.RDAPCheck)
	mux.HandleFunc("GET /rdap", app.RDAPLog)
	mux.HandleFunc("POST /assets/{id}/documents", app.DocumentUpload)
	mux.HandleFunc("GET /documents/{id}", app.DocumentServe)
	mux.HandleFunc("POST /documents/{id}/delete", app.DocumentDelete)
	mux.HandleFunc("GET /finance", app.FinancePage)
	mux.HandleFunc("POST /assets/{id}/credentials", app.CredentialCreate)
	mux.HandleFunc("POST /assets/{id}/credentials/{cid}", app.CredentialUpdate)
	mux.HandleFunc("POST /assets/{id}/credentials/{cid}/delete", app.CredentialDelete)
	mux.HandleFunc("POST /assets/{id}/credentials/{cid}/reveal", app.CredentialReveal)
	mux.HandleFunc("POST /assets/{id}/credentials/{cid}/copy", app.CredentialCopy)

	mux.HandleFunc("GET /projects", app.ProjectList)
	mux.HandleFunc("GET /projects/new", app.ProjectNew)
	mux.HandleFunc("POST /projects", app.ProjectCreate)
	mux.HandleFunc("GET /projects/{id}", app.ProjectShow)
	mux.HandleFunc("GET /projects/{id}/edit", app.ProjectEdit)
	mux.HandleFunc("POST /projects/{id}", app.ProjectUpdate)
	mux.HandleFunc("POST /projects/{id}/delete", app.ProjectDelete)
	mux.HandleFunc("GET /projects/{id}/screenshot", app.ProjectScreenshotImage)
	mux.HandleFunc("GET /projects/{id}/screenshot/status", app.ProjectScreenshotStatus)
	mux.HandleFunc("POST /projects/{id}/screenshot", app.ProjectScreenshotRefresh)

	mux.HandleFunc("GET /clients", app.ClientList)
	mux.HandleFunc("GET /clients/new", app.ClientNew)
	mux.HandleFunc("POST /clients", app.ClientCreate)
	mux.HandleFunc("GET /clients/{id}", app.ClientShow)
	mux.HandleFunc("GET /clients/{id}/edit", app.ClientEdit)
	mux.HandleFunc("POST /clients/{id}", app.ClientUpdate)
	mux.HandleFunc("POST /clients/{id}/delete", app.ClientDelete)

	mux.HandleFunc("GET /settings/providers", app.ProviderList)
	mux.HandleFunc("POST /settings/providers", app.ProviderCreate)
	mux.HandleFunc("GET /settings/providers/{id}", app.ProviderEdit)
	mux.HandleFunc("POST /settings/providers/{id}", app.ProviderUpdate)
	mux.HandleFunc("POST /settings/providers/{id}/delete", app.ProviderDelete)

	mux.HandleFunc("GET /settings/types", app.TypeList)
	mux.HandleFunc("GET /settings/types/new", app.TypeNew)
	mux.HandleFunc("POST /settings/types", app.TypeCreate)
	mux.HandleFunc("GET /settings/types/{id}", app.TypeEdit)
	mux.HandleFunc("POST /settings/types/{id}", app.TypeUpdate)
	mux.HandleFunc("POST /settings/types/{id}/delete", app.TypeDelete)
	mux.HandleFunc("POST /settings/types/{id}/fields", app.FieldCreate)
	mux.HandleFunc("POST /settings/types/{id}/fields/{fid}", app.FieldUpdate)
	mux.HandleFunc("GET /settings/types/{id}/fields/{fid}/remove", app.FieldRemoveConfirm)
	mux.HandleFunc("POST /settings/types/{id}/fields/{fid}/remove", app.FieldRemove)

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
