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

type telegramForm struct {
	Settings settings.Telegram
	Token    string
	Errors   map[string]string
}

func parseTelegramForm(r *http.Request, saved settings.Telegram) telegramForm {
	f := telegramForm{
		Settings: settings.Telegram{
			Enabled:  r.PostFormValue("enabled") != "",
			ChatID:   strings.TrimSpace(r.PostFormValue("chat_id")),
			HasToken: saved.HasToken,
		},
		Token:  strings.TrimSpace(r.PostFormValue("bot_token")),
		Errors: map[string]string{},
	}
	if raw := strings.TrimSpace(r.PostFormValue("topic_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			f.Errors["topic_id"] = "Topic ID must be a positive whole number."
		}
		f.Settings.TopicID = id
	}
	if f.Token != "" && !strings.Contains(f.Token, ":") {
		f.Errors["bot_token"] = "Enter the token exactly as BotFather shows it, e.g. 123456789:ABC…"
	}
	if f.Settings.ChatID != "" && !validChatID(f.Settings.ChatID) {
		f.Errors["chat_id"] = "Use a numeric chat ID (groups start with -100) or a @channelname."
	}
	return f
}

func validChatID(s string) bool {
	if strings.HasPrefix(s, "@") {
		return len(s) > 1 && !strings.ContainsAny(s, " \t")
	}
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func (f telegramForm) requireComplete() {
	if f.Settings.ChatID == "" {
		f.Errors["chat_id"] = "Chat ID is required."
	}
	if f.Token == "" && !f.Settings.HasToken {
		f.Errors["bot_token"] = "Bot token is required."
	}
}

type emailForm struct {
	Settings settings.Email
	Password string
	Errors   map[string]string
}

func parseEmailForm(r *http.Request, saved settings.Email) emailForm {
	port, perr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("port")))
	f := emailForm{
		Settings: settings.Email{
			Enabled:     r.PostFormValue("enabled") != "",
			Host:        strings.TrimSpace(r.PostFormValue("host")),
			Port:        port,
			Username:    strings.TrimSpace(r.PostFormValue("username")),
			From:        strings.TrimSpace(r.PostFormValue("from")),
			FromName:    strings.TrimSpace(r.PostFormValue("from_name")),
			To:          strings.TrimSpace(r.PostFormValue("to")),
			HasPassword: saved.HasPassword,
		},
		Password: r.PostFormValue("password"),
		Errors:   map[string]string{},
	}
	if perr != nil || port < 1 || port > 65535 {
		f.Errors["port"] = "Enter a port between 1 and 65535, usually 587 or 465."
		f.Settings.Port = 587
	}
	if f.Settings.From != "" && !plainAddress(f.Settings.From) {
		f.Errors["from"] = "Enter only the email address; put the name in From name."
	}
	if f.Settings.To != "" && !plainAddress(f.Settings.To) {
		f.Errors["to"] = "Enter a valid email address."
	}
	if strings.ContainsAny(f.Settings.FromName, "<>\"\r\n") {
		f.Errors["from_name"] = "From name cannot contain < > \" or line breaks."
	}
	return f
}

func plainAddress(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

func (f emailForm) requireComplete() {
	if f.Settings.Host == "" {
		f.Errors["host"] = "SMTP host is required."
	}
	if f.Settings.From == "" {
		f.Errors["from"] = "From address is required."
	}
	if f.Settings.To == "" {
		f.Errors["to"] = "To address is required."
	}
}

func (a *App) SettingsTelegramSave(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := parseTelegramForm(r, all.Telegram)
	if f.Settings.Enabled {
		f.requireComplete()
	}
	if len(f.Errors) > 0 {
		a.renderNotifications(w, r, http.StatusUnprocessableEntity, notificationsData{Telegram: f.Settings, Email: all.Email, TGErrors: f.Errors})
		return
	}
	if err := a.Settings.SaveTelegram(r.Context(), f.Settings, f.Token); err != nil {
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
	f := parseEmailForm(r, all.Email)
	if f.Settings.Enabled {
		f.requireComplete()
	}
	if len(f.Errors) > 0 {
		a.renderNotifications(w, r, http.StatusUnprocessableEntity, notificationsData{Telegram: all.Telegram, Email: f.Settings, MailErrs: f.Errors})
		return
	}
	if err := a.Settings.SaveEmail(r.Context(), f.Settings, f.Password); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/notifications", "Email settings saved.", false)
}

func firstMessage(errs map[string]string, order ...string) string {
	for _, k := range order {
		if m, ok := errs[k]; ok {
			return m
		}
	}
	return "Please check the form."
}

func (a *App) runTest(w http.ResponseWriter, r *http.Request, channel string, send func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := send(ctx); err != nil {
		a.Logger.Warn("test notification failed", "channel", channel, "error", err.Error())
		toastHeader(w, "Test "+channel+" failed: "+err.Error(), true)
		w.WriteHeader(http.StatusOK)
		return
	}
	toastHeader(w, "Test "+channel+" sent. Settings are not saved until you press Save.", false)
	w.WriteHeader(http.StatusOK)
}

func (a *App) SettingsTelegramTest(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := parseTelegramForm(r, all.Telegram)
	f.requireComplete()
	if len(f.Errors) > 0 {
		toastHeader(w, firstMessage(f.Errors, "bot_token", "chat_id", "topic_id"), true)
		w.WriteHeader(http.StatusOK)
		return
	}
	a.runTest(w, r, "Telegram message", func(ctx context.Context) error {
		token := f.Token
		if token == "" {
			if token, err = a.Settings.Secret(ctx, settings.SecretTelegramBotToken); err != nil {
				return err
			}
		}
		cfg := telegram.Config{Token: token, ChatID: f.Settings.ChatID, TopicID: f.Settings.TopicID}
		return a.Telegram.Send(ctx, cfg, notify.TestMessage("Telegram"))
	})
}

func (a *App) SettingsEmailTest(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := parseEmailForm(r, all.Email)
	f.requireComplete()
	if len(f.Errors) > 0 {
		toastHeader(w, firstMessage(f.Errors, "host", "port", "from", "from_name", "to"), true)
		w.WriteHeader(http.StatusOK)
		return
	}
	a.runTest(w, r, "email", func(ctx context.Context) error {
		pw := f.Password
		if pw == "" {
			if pw, err = a.Settings.Secret(ctx, settings.SecretSMTPPassword); err != nil {
				return err
			}
		}
		e := f.Settings
		cfg := email.Config{Host: e.Host, Port: e.Port, Username: e.Username, Password: pw, From: e.From, FromName: e.FromName, To: e.To}
		return a.Email.Send(ctx, cfg, notify.TestMessage("email"))
	})
}
