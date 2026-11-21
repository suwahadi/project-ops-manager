package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/auth"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/settings"
	"github.com/suwahadi/project-ops-manager/internal/web/middleware"
)

type settingsTab struct {
	Key   string
	Label string
	Href  string
}

var settingsTabs = []settingsTab{
	{"account", "Account", "/settings"},
	{"general", "General", "/settings/general"},
	{"notifications", "Notifications", "/settings/notifications"},
	{"invoice", "Invoice", "/settings/invoice"},
	{"types", "Asset types", "/settings/types"},
	{"providers", "Providers", "/settings/providers"},
}

type settingsShell struct {
	Tabs   []settingsTab
	Active string
	Body   any
}

func shell(active string, body any) settingsShell {
	return settingsShell{Tabs: settingsTabs, Active: active, Body: body}
}

type accountForm struct {
	Email       string
	EmailErrors FieldErrors
	PassErrors  FieldErrors
}

func (a *App) SettingsAccount(w http.ResponseWriter, r *http.Request) {
	sess, _ := middleware.SessionFrom(r.Context())
	user, err := a.Auth.User(r.Context(), sess.UserID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "settings_account", "Settings", "settings", shell("account", accountForm{Email: user.Email}))
}

func (a *App) SettingsAccountEmail(w http.ResponseWriter, r *http.Request) {
	sess, _ := middleware.SessionFrom(r.Context())
	email := strings.TrimSpace(r.PostFormValue("email"))
	errs := FieldErrors{}
	if _, err := mail.ParseAddress(email); err != nil || email == "" {
		errs.Add("email", "Enter a valid email address.")
	}
	if errs.Any() {
		a.render(w, r, http.StatusUnprocessableEntity, "settings_account", "Settings", "settings", shell("account", accountForm{Email: email, EmailErrors: errs}))
		return
	}
	if err := a.Auth.ChangeEmail(r.Context(), sess.UserID, email); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings", "Email updated.", false)
}

func (a *App) SettingsAccountPassword(w http.ResponseWriter, r *http.Request) {
	sess, _ := middleware.SessionFrom(r.Context())
	user, err := a.Auth.User(r.Context(), sess.UserID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	current := r.PostFormValue("current_password")
	next := r.PostFormValue("new_password")
	confirm := r.PostFormValue("confirm_password")
	errs := FieldErrors{}
	if err := auth.ValidateNewPassword(next); err != nil {
		errs.Add("new_password", err.Error())
	}
	if next != confirm {
		errs.Add("confirm_password", "Passwords do not match.")
	}
	if !errs.Any() {
		err := a.Auth.ChangePassword(r.Context(), sess, current, next)
		switch {
		case errors.Is(err, auth.ErrWrongPassword):
			errs.Add("current_password", "Current password is incorrect.")
		case err != nil:
			a.serverError(w, r, err)
			return
		}
	}
	if errs.Any() {
		a.render(w, r, http.StatusUnprocessableEntity, "settings_account", "Settings", "settings", shell("account", accountForm{Email: user.Email, PassErrors: errs}))
		return
	}
	a.redirectWithFlash(w, r, "/settings", "Password changed. Other sessions were signed out.", false)
}

type generalForm struct {
	Timezone   string
	SendTime   string
	Currency   string
	Currencies []money.Currency
	Errors     FieldErrors
}

func (a *App) SettingsGeneral(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	g := all.General
	a.render(w, r, http.StatusOK, "settings_general", "Settings", "settings", shell("general", generalForm{
		Timezone: g.Timezone, SendTime: g.SendTime, Currency: string(g.DefaultCurrency), Currencies: money.Currencies,
	}))
}

func (a *App) SettingsGeneralSave(w http.ResponseWriter, r *http.Request) {
	f := generalForm{
		Timezone:   strings.TrimSpace(r.PostFormValue("timezone")),
		SendTime:   strings.TrimSpace(r.PostFormValue("send_time")),
		Currency:   r.PostFormValue("default_currency"),
		Currencies: money.Currencies,
		Errors:     FieldErrors{},
	}
	if _, err := time.LoadLocation(f.Timezone); err != nil || f.Timezone == "" {
		f.Errors.Add("timezone", "Enter a valid IANA timezone, e.g. Asia/Jakarta.")
	}
	if _, _, err := settings.ParseClock(f.SendTime); err != nil {
		f.Errors.Add("send_time", "Use the 24-hour format HH:MM.")
	}
	cur, err := money.ParseCurrency(f.Currency)
	if err != nil {
		f.Errors.Add("default_currency", "Choose IDR or USD.")
	}
	if f.Errors.Any() {
		a.render(w, r, http.StatusUnprocessableEntity, "settings_general", "Settings", "settings", shell("general", f))
		return
	}
	if err := a.Settings.SaveGeneral(r.Context(), settings.General{Timezone: f.Timezone, SendTime: f.SendTime, DefaultCurrency: cur}); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/general", "Settings saved.", false)
}
