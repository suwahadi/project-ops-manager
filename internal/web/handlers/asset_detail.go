package handlers

import (
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/credential"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/reminder"
)

type assetTab struct {
	Key   string
	Label string
}

var assetTabs = []assetTab{
	{"overview", "Overview"},
	{"renewals", "Renewals"},
	{"documents", "Documents"},
	{"notes", "Notes & Credentials"},
	{"reminders", "Reminders"},
	{"projects", "Projects"},
}

type customFieldView struct {
	Label string
	Value string
	URL   bool
}

type assetDetailData struct {
	Asset        asset.Detail
	Tabs         []assetTab
	Tab          string
	Location     *time.Location
	CustomFields []customFieldView
	NotesHTML    template.HTML
	Credentials  []sqlc.ListCredentialsRow
	Extra        any
	Renew        renewDialogData
}

func (a *App) AssetShow(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	tab := r.URL.Query().Get("tab")
	valid := false
	for _, t := range assetTabs {
		if t.Key == tab {
			valid = true
		}
	}
	if !valid {
		tab = "overview"
	}
	data := assetDetailData{Asset: det, Tabs: assetTabs, Tab: tab, Location: a.location(r)}
	data.Renew = renewDialogData{Asset: det, Form: a.defaultRenewForm(det, a.today(r))}
	ctx := r.Context()
	switch tab {
	case "overview":
		fields, err := a.AssetTypes.Fields(ctx, det.AssetTypeID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		for _, f := range fields {
			v := det.Custom.Display(f)
			if v == "" {
				continue
			}
			data.CustomFields = append(data.CustomFields, customFieldView{Label: f.Label, Value: v, URL: f.Type == assettype.FieldURL})
		}
	case "notes":
		data.NotesHTML = a.Markdown.Render(det.Notes)
		creds, err := a.Credentials.List(ctx, det.ID)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		data.Credentials = creds
	}
	if err := a.loadAssetTabExtra(r, &data); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "asset_detail", det.Name, "assets", data)
}

func (a *App) loadAssetTabExtra(r *http.Request, d *assetDetailData) error {
	switch d.Tab {
	case "renewals":
		extra, err := a.renewalsTab(r, d.Asset)
		if err != nil {
			return err
		}
		d.Extra = extra
	case "documents":
		own, _, err := a.Documents.ListForAsset(r.Context(), d.Asset.ID)
		if err != nil {
			return err
		}
		d.Extra = own
	case "projects":
		rows, err := a.Projects.ByAsset(r.Context(), d.Asset.ID)
		if err != nil {
			return err
		}
		d.Extra = rows
	case "reminders":
		extra, err := a.reminderTab(r, d.Asset)
		if err != nil {
			return err
		}
		d.Extra = extra
	}
	return nil
}

type reminderCell struct {
	Channel string
	Status  string
	Class   string
}

type reminderPlanRow struct {
	Offset int32
	Due    time.Time
	Cells  []reminderCell
}

type reminderTabData struct {
	Plan     []reminderPlanRow
	Logs     []sqlc.ReminderLog
	Channels []string
	SendTime string
}

func (a *App) reminderTab(r *http.Request, det asset.Detail) (reminderTabData, error) {
	ctx := r.Context()
	all, err := a.Settings.Load(ctx)
	if err != nil {
		return reminderTabData{}, err
	}
	logs, err := a.Queries.ListReminderLogsByAsset(ctx, det.ID)
	if err != nil {
		return reminderTabData{}, err
	}
	var channels []string
	if all.Telegram.Enabled {
		channels = append(channels, reminder.ChannelTelegram)
	}
	if all.Email.Enabled {
		channels = append(channels, reminder.ChannelEmail)
	}
	today := asset.Today(a.Now(), all.General.Location())
	data := reminderTabData{Logs: logs, Channels: channels, SendTime: all.General.SendTime}
	for _, p := range reminder.Plan(det.ExpirationDate, det.ReminderOffsets) {
		row := reminderPlanRow{Offset: p.Offset, Due: p.Due}
		for _, ch := range channels {
			cell := reminderCell{Channel: ch}
			var found *sqlc.ReminderLog
			for i := range logs {
				l := logs[i]
				if l.Channel == ch && l.OffsetDays == p.Offset && l.ExpirationDate.Equal(det.ExpirationDate) {
					found = &logs[i]
				}
			}
			switch {
			case found != nil && found.Status == "sent":
				cell.Status, cell.Class = "Sent", "badge-active"
			case found != nil && found.Status == "failed":
				cell.Status, cell.Class = "Failed", "badge-expired"
			case found != nil:
				cell.Status, cell.Class = "Sending", "badge-expiring"
			case det.CancelledAt != nil:
				cell.Status, cell.Class = "Stopped", "badge-cancelled"
			case p.Due.Before(today):
				cell.Status, cell.Class = "Not sent", "badge-cancelled"
			default:
				cell.Status, cell.Class = "Scheduled", "badge-outline"
			}
			row.Cells = append(row.Cells, cell)
		}
		data.Plan = append(data.Plan, row)
	}
	return data, nil
}

func credentialInput(r *http.Request) (credential.Input, map[string]string) {
	in := credential.Input{
		Label:    strings.TrimSpace(r.PostFormValue("label")),
		URL:      strings.TrimSpace(r.PostFormValue("url")),
		Username: r.PostFormValue("username"),
		Password: r.PostFormValue("password"),
		Notes:    r.PostFormValue("notes"),
	}
	errs := map[string]string{}
	if in.Label == "" {
		errs["label"] = "Label is required."
	}
	if in.URL != "" && !assettype.ValidURL(in.URL) {
		errs["url"] = "Enter a valid http or https URL."
	}
	return in, errs
}

func firstError(errs map[string]string) string {
	for _, k := range []string{"label", "url"} {
		if v, ok := errs[k]; ok {
			return v
		}
	}
	return ""
}

func (a *App) CredentialCreate(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	back := "/assets/" + det.ID + "?tab=notes"
	in, errs := credentialInput(r)
	if len(errs) > 0 {
		a.redirectWithFlash(w, r, back, firstError(errs), true)
		return
	}
	if err := a.Credentials.Create(r.Context(), det.ID, in); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, back, "Credential added.", false)
}

func (a *App) CredentialUpdate(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	back := "/assets/" + det.ID + "?tab=notes"
	cid := r.PathValue("cid")
	if _, err := a.Credentials.Get(r.Context(), det.ID, cid); err != nil {
		a.notFound(w, r)
		return
	}
	in, errs := credentialInput(r)
	if len(errs) > 0 {
		a.redirectWithFlash(w, r, back, firstError(errs), true)
		return
	}
	change := in.Password != "" || r.PostFormValue("clear_password") != ""
	if r.PostFormValue("clear_password") != "" {
		in.Password = ""
	}
	if err := a.Credentials.Update(r.Context(), det.ID, cid, in, change); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, back, "Credential saved.", false)
}

func (a *App) CredentialDelete(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	if err := a.Credentials.Delete(r.Context(), det.ID, r.PathValue("cid")); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID+"?tab=notes", "Credential deleted.", false)
}

func (a *App) revealPassword(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, cid := r.PathValue("id"), r.PathValue("cid")
	if !isUUID(id) || !isUUID(cid) {
		http.NotFound(w, r)
		return "", false
	}
	pw, err := a.Credentials.Reveal(r.Context(), id, cid)
	if errors.Is(err, credential.ErrNotFound) || errors.Is(err, credential.ErrNoPassword) {
		http.NotFound(w, r)
		return "", false
	}
	if err != nil {
		a.Logger.Error("reveal credential failed", "credential_id", cid)
		http.Error(w, "Could not decrypt the password.", http.StatusInternalServerError)
		return "", false
	}
	w.Header().Set("Cache-Control", "no-store")
	return pw, true
}

func (a *App) CredentialReveal(w http.ResponseWriter, r *http.Request) {
	pw, ok := a.revealPassword(w, r)
	if !ok {
		return
	}
	a.fragment(w, r, http.StatusOK, "asset_detail", "revealed_password", pw)
}

func (a *App) CredentialCopy(w http.ResponseWriter, r *http.Request) {
	pw, ok := a.revealPassword(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(pw))
}

type renewalView struct {
	sqlc.Renewal
	Documents []sqlc.Document
}

func (a *App) renewalsTab(r *http.Request, det asset.Detail) ([]renewalView, error) {
	rows, err := a.Assets.Renewals(r.Context(), det.ID)
	if err != nil {
		return nil, err
	}
	_, docs, err := a.Documents.ListForAsset(r.Context(), det.ID)
	if err != nil {
		return nil, err
	}
	out := make([]renewalView, len(rows))
	for i, rw := range rows {
		out[i] = renewalView{Renewal: rw}
		for _, d := range docs {
			if d.RenewalID != nil && *d.RenewalID == rw.ID {
				out[i].Documents = append(out[i].Documents, d)
			}
		}
	}
	return out, nil
}
