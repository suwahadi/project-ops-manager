package handlers

import (
	"context"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/notify"
	"github.com/suwahadi/project-ops-manager/internal/notify/email"
	"github.com/suwahadi/project-ops-manager/internal/notify/telegram"
	"github.com/suwahadi/project-ops-manager/internal/settings"
)

type notificationsData struct {
	Telegram settings.Telegram
	Email    settings.Email
	TGErrors map[string]string
	MailErrs map[string]string
}

func (a *App) renderNotifications(w http.ResponseWriter, r *http.Request, status int, d notificationsData) {
	if d.TGErrors == nil {
		d.TGErrors = map[string]string{}
	}
	if d.MailErrs == nil {
		d.MailErrs = map[string]string{}
	}
	a.render(w, r, status, "settings_notifications", "Settings", "settings", shell("notifications", d))
}

func (a *App) SettingsNotifications(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.renderNotifications(w, r, http.StatusOK, notificationsData{Telegram: all.Telegram, Email: all.Email})
}

func (a *App) SettingsTelegramSave(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	t := settings.Telegram{Enabled: r.PostFormValue("enabled") != "", ChatID: strings.TrimSpace(r.PostFormValue("chat_id")), HasToken: all.Telegram.HasToken}
	token := strings.TrimSpace(r.PostFormValue("bot_token"))
	errs := map[string]string{}
	if t.Enabled && t.ChatID == "" {
		errs["chat_id"] = "Chat ID is required when Telegram is enabled."
	}
	if t.Enabled && token == "" && !all.Telegram.HasToken {
		errs["bot_token"] = "Bot token is required when Telegram is enabled."
	}
	if len(errs) > 0 {
		a.renderNotifications(w, r, http.StatusUnprocessableEntity, notificationsData{Telegram: t, Email: all.Email, TGErrors: errs})
		return
	}
	if err := a.Settings.SaveTelegram(r.Context(), t, token); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/notifications", "Telegram settings saved.", false)
}

func (a *App) SettingsEmailSave(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	port, perr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("port")))
	e := settings.Email{
		Enabled: r.PostFormValue("enabled") != "", Host: strings.TrimSpace(r.PostFormValue("host")), Port: port,
		Username: strings.TrimSpace(r.PostFormValue("username")), From: strings.TrimSpace(r.PostFormValue("from")),
		To: strings.TrimSpace(r.PostFormValue("to")), HasPassword: all.Email.HasPassword,
	}
	password := r.PostFormValue("password")
	errs := map[string]string{}
	if perr != nil || port < 1 || port > 65535 {
		errs["port"] = "Enter a port between 1 and 65535, usually 587 or 465."
		e.Port = 587
	}
	if e.From != "" {
		if _, err := mail.ParseAddress(e.From); err != nil {
			errs["from"] = "Enter a valid from address."
		}
	}
	if e.To != "" {
		if _, err := mail.ParseAddress(e.To); err != nil {
			errs["to"] = "Enter a valid to address."
		}
	}
	if e.Enabled {
		if e.Host == "" {
			errs["host"] = "SMTP host is required when email is enabled."
		}
		if e.From == "" {
			errs["from"] = "From address is required when email is enabled."
		}
		if e.To == "" {
			errs["to"] = "To address is required when email is enabled."
		}
	}
	if len(errs) > 0 {
		a.renderNotifications(w, r, http.StatusUnprocessableEntity, notificationsData{Telegram: all.Telegram, Email: e, MailErrs: errs})
		return
	}
	if err := a.Settings.SaveEmail(r.Context(), e, password); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/notifications", "Email settings saved.", false)
}

func (a *App) testSend(w http.ResponseWriter, r *http.Request, channel string, send func(ctx context.Context, all settings.All) error) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := send(ctx, all); err != nil {
		a.Logger.Warn("test notification failed", "channel", channel, "error", err.Error())
		toastHeader(w, "Test "+channel+" failed: "+err.Error(), true)
		w.WriteHeader(http.StatusOK)
		return
	}
	toastHeader(w, "Test "+channel+" sent.", false)
	w.WriteHeader(http.StatusOK)
}

func (a *App) SettingsTelegramTest(w http.ResponseWriter, r *http.Request) {
	a.testSend(w, r, "Telegram message", func(ctx context.Context, all settings.All) error {
		token, err := a.Settings.Secret(ctx, settings.SecretTelegramBotToken)
		if err != nil {
			return err
		}
		return a.Telegram.Send(ctx, telegram.Config{Token: token, ChatID: all.Telegram.ChatID}, notify.TestMessage("Telegram"))
	})
}

func (a *App) SettingsEmailTest(w http.ResponseWriter, r *http.Request) {
	a.testSend(w, r, "email", func(ctx context.Context, all settings.All) error {
		pw, err := a.Settings.Secret(ctx, settings.SecretSMTPPassword)
		if err != nil {
			return err
		}
		e := all.Email
		return a.Email.Send(ctx, email.Config{Host: e.Host, Port: e.Port, Username: e.Username, Password: pw, From: e.From, To: e.To}, notify.TestMessage("email"))
	})
}
