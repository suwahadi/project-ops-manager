package handlers

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/project"
	"github.com/suwahadi/project-ops-manager/internal/screenshot"
)

type projectFormData struct {
	IsEdit   bool
	ID       string
	Name     string
	Owner    string
	URL      string
	Brief    string
	AssetIDs []string
	Errors   map[string]string
	Clients  []sqlc.ListClientOptionsRow
	Assets   []sqlc.ListAssetOptionsRow
}

func (a *App) loadProjectFormOptions(r *http.Request, d *projectFormData) error {
	var err error
	if d.Clients, err = a.Clients.Options(r.Context()); err != nil {
		return err
	}
	if d.Assets, err = a.Assets.Options(r.Context()); err != nil {
		return err
	}
	if d.Errors == nil {
		d.Errors = map[string]string{}
	}
	return nil
}

func (a *App) ProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Projects.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "projects_list", "Projects", "projects", rows)
}

func (a *App) ProjectNew(w http.ResponseWriter, r *http.Request) {
	d := projectFormData{Owner: "internal", Brief: project.BriefTemplate}
	if o := r.URL.Query().Get("owner"); isUUID(o) {
		d.Owner = o
	}
	if err := a.loadProjectFormOptions(r, &d); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "project_form", "New project", "projects", d)
}

func (a *App) parseProjectForm(r *http.Request) (project.Input, projectFormData) {
	_ = r.ParseForm()
	d := projectFormData{
		Name:   strings.TrimSpace(r.PostFormValue("name")),
		Owner:  r.PostFormValue("owner"),
		URL:    strings.TrimSpace(r.PostFormValue("url")),
		Brief:  strings.ReplaceAll(r.PostFormValue("brief"), "\r\n", "\n"),
		Errors: map[string]string{},
	}
	in := project.Input{Name: d.Name, Brief: d.Brief}
	if d.Name == "" {
		d.Errors["name"] = "Name is required."
	}
	if d.Owner != "" && d.Owner != "internal" {
		if !isUUID(d.Owner) {
			d.Errors["owner"] = "Choose Internal or an existing client."
		} else if _, err := a.Clients.Get(r.Context(), d.Owner); err != nil {
			d.Errors["owner"] = "Choose Internal or an existing client."
		} else {
			id := d.Owner
			in.ClientID = &id
		}
	}
	if d.URL != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		err := a.Guard.CheckURL(ctx, d.URL)
		cancel()
		switch {
		case errors.Is(err, screenshot.ErrScheme):
			d.Errors["url"] = "Use an http or https URL."
		case errors.Is(err, screenshot.ErrPrivateHost):
			d.Errors["url"] = "Private, loopback and link-local addresses cannot be captured."
		case err != nil && strings.Contains(err.Error(), "not valid"):
			d.Errors["url"] = "Enter a valid URL."
		}
		u := d.URL
		in.URL = &u
	}
	for _, id := range r.PostForm["asset_ids"] {
		if isUUID(id) {
			in.AssetIDs = append(in.AssetIDs, id)
		}
	}
	d.AssetIDs = in.AssetIDs
	return in, d
}

func (a *App) ProjectCreate(w http.ResponseWriter, r *http.Request) {
	in, d := a.parseProjectForm(r)
	if len(d.Errors) > 0 {
		if err := a.loadProjectFormOptions(r, &d); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusUnprocessableEntity, "project_form", "New project", "projects", d)
		return
	}
	id, err := a.Projects.Create(r.Context(), in)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	msg := "Project created."
	if in.URL != nil {
		msg = "Project created. Capturing a screenshot…"
	}
	a.redirectWithFlash(w, r, "/projects/"+id, msg, false)
}

func (a *App) loadProject(w http.ResponseWriter, r *http.Request) (sqlc.GetProjectRow, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return sqlc.GetProjectRow{}, false
	}
	p, err := a.Projects.Get(r.Context(), id)
	if errors.Is(err, project.ErrNotFound) {
		a.notFound(w, r)
		return p, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return p, false
	}
	return p, true
}

func (a *App) ProjectEdit(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	linked, err := a.Projects.Assets(r.Context(), p.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := projectFormData{IsEdit: true, ID: p.ID, Name: p.Name, Owner: "internal", URL: deref(p.Url), Brief: p.Brief}
	if p.ClientID != nil {
		d.Owner = *p.ClientID
	}
	for _, l := range linked {
		d.AssetIDs = append(d.AssetIDs, l.ID)
	}
	if err := a.loadProjectFormOptions(r, &d); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "project_form", "Edit "+p.Name, "projects", d)
}

func (a *App) ProjectUpdate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	in, d := a.parseProjectForm(r)
	d.IsEdit, d.ID = true, p.ID
	if len(d.Errors) > 0 {
		if err := a.loadProjectFormOptions(r, &d); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusUnprocessableEntity, "project_form", "Edit "+p.Name, "projects", d)
		return
	}
	if err := a.Projects.Update(r.Context(), p.ID, in); err != nil {
		a.serverError(w, r, err)
		return
	}
	msg := "Project saved."
	if in.URL != nil && (p.Url == nil || *p.Url != *in.URL) {
		msg = "Project saved. Capturing a new screenshot…"
	}
	a.redirectWithFlash(w, r, "/projects/"+p.ID, msg, false)
}

func (a *App) ProjectDelete(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	paths, err := a.Projects.Delete(r.Context(), p.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Documents.Remove(paths...)
	a.redirectWithFlash(w, r, "/projects", "Project deleted.", false)
}

type projectAssetView struct {
	sqlc.ListProjectAssetsRow
	Status    asset.Status
	Countdown string
}

type projectDetailData struct {
	Project   sqlc.GetProjectRow
	BriefHTML template.HTML
	Assets    []projectAssetView
	Shot      screenshotView
}

type screenshotView struct {
	ProjectID  string
	HasURL     bool
	Status     string
	HasImage   bool
	CapturedAt string
	Error      string
	Version    int64
}

func (a *App) screenshotView(r *http.Request, p sqlc.GetProjectRow) (screenshotView, error) {
	v := screenshotView{ProjectID: p.ID, HasURL: p.Url != nil}
	sc, err := a.Projects.Screenshot(r.Context(), p.ID)
	if err != nil || sc == nil {
		return v, err
	}
	v.Status = sc.Status
	v.HasImage = sc.StoredPath != nil
	if sc.CapturedAt != nil {
		v.CapturedAt = sc.CapturedAt.In(a.location(r)).Format("02 Jan 2006 15:04")
		v.Version = sc.CapturedAt.Unix()
	}
	if sc.Error != nil {
		v.Error = *sc.Error
	}
	return v, nil
}

func (a *App) ProjectShow(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	linked, err := a.Projects.Assets(r.Context(), p.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	shot, err := a.screenshotView(r, p)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	today := a.today(r)
	d := projectDetailData{Project: p, BriefHTML: a.Markdown.Render(p.Brief), Shot: shot}
	for _, l := range linked {
		d.Assets = append(d.Assets, projectAssetView{ListProjectAssetsRow: l,
			Status: asset.ComputeStatus(l.CancelledAt != nil, l.ExpirationDate, today, l.ReminderOffsets), Countdown: asset.Countdown(l.ExpirationDate, today)})
	}
	a.render(w, r, http.StatusOK, "project_detail", p.Name, "projects", d)
}

func (a *App) ProjectScreenshotStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	shot, err := a.screenshotView(r, p)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.fragment(w, r, http.StatusOK, "project_detail", "screenshot_panel", shot)
}

func (a *App) ProjectScreenshotRefresh(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	if p.Url == nil {
		a.redirectWithFlash(w, r, "/projects/"+p.ID, "Add a website URL first.", true)
		return
	}
	if err := a.Projects.RequestScreenshot(r.Context(), p.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	if isHTMX(r) {
		shot, err := a.screenshotView(r, p)
		if err != nil {
			a.serverError(w, r, err)
			return
		}
		a.fragment(w, r, http.StatusOK, "project_detail", "screenshot_panel", shot)
		return
	}
	a.redirectWithFlash(w, r, "/projects/"+p.ID, "Capturing a new screenshot…", false)
}

func (a *App) ProjectScreenshotImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		http.NotFound(w, r)
		return
	}
	sc, err := a.Projects.Screenshot(r.Context(), id)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if sc == nil || sc.StoredPath == nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(a.Documents.Path(*sc.StoredPath))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Disposition", `inline; filename="screenshot.webp"`)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "", st.ModTime(), f)
}
