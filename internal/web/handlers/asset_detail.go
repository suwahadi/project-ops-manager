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
)

type assetTab struct {
	Key   string
	Label string
}

var assetTabs = []assetTab{
	{"overview", "Overview"},
	{"notes", "Notes & Credentials"},
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
	return nil
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
