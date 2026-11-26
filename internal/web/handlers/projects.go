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
	"github.com/suwahadi/project-ops-manager/internal/invoice"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/project"
	"github.com/suwahadi/project-ops-manager/internal/screenshot"
)

type projectFormData struct {
	IsEdit      bool
	ID          string
	Name        string
	Owner       string
	URL         string
	Brief       string
	AssetIDs    []string
	Currency    string
	Value       string
	Cost        string
	BillingDate string
	Errors      map[string]string
	Clients     []sqlc.ListClientOptionsRow
	Assets      []sqlc.ListAssetOptionsRow
	Currencies  []money.Currency
}

func (a *App) loadProjectFormOptions(r *http.Request, d *projectFormData) error {
	var err error
	if d.Clients, err = a.Clients.Options(r.Context()); err != nil {
		return err
	}
	if d.Assets, err = a.Assets.Options(r.Context()); err != nil {
		return err
	}
	d.Currencies = money.Currencies
	if d.Currency == "" {
		all, err := a.Settings.Load(r.Context())
		if err != nil {
			return err
		}
		d.Currency = string(all.General.DefaultCurrency)
	}
	if d.Errors == nil {
		d.Errors = map[string]string{}
	}
	return nil
}

type projectListData struct {
	Filter            project.Filter
	Items             []sqlc.ListProjectsRow
	Clients           []sqlc.ListClientOptionsRow
	ScreenshotOptions []project.Option
	PaymentOptions    []project.Option
	SortOptions       []project.Option
}

func (a *App) ProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Projects.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := project.ParseFilter(r.URL.Query(), isUUID)
	data := projectListData{Filter: f, Items: project.Apply(rows, f), ScreenshotOptions: project.ScreenshotOptions, PaymentOptions: project.PaymentOptions, SortOptions: project.SortOptions}
	if isHTMX(r) && r.Header.Get("HX-Target") == "project-results" {
		a.fragment(w, r, http.StatusOK, "projects_list", "project_results", data)
		return
	}
	if data.Clients, err = a.Clients.Options(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "projects_list", "Projects", "projects", data)
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
	a.parseProjectMoney(r, &in, &d)
	return in, d
}

func optionalAmount(raw string) (money.NullAmount, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return money.NullAmount{}, true
	}
	v, err := money.Parse(raw)
	if err != nil || v < 0 {
		return money.NullAmount{}, false
	}
	return money.Some(v), true
}

func (a *App) parseProjectMoney(r *http.Request, in *project.Input, d *projectFormData) {
	d.Currency = r.PostFormValue("currency")
	d.Value = strings.TrimSpace(r.PostFormValue("project_value"))
	d.Cost = strings.TrimSpace(r.PostFormValue("project_cost"))
	d.BillingDate = strings.TrimSpace(r.PostFormValue("billing_date"))
	cur, err := money.ParseCurrency(d.Currency)
	if err != nil {
		cur = money.IDR
		if d.Currency != "" {
			d.Errors["currency"] = "Choose IDR or USD."
		}
	}
	in.Currency = cur
	if in.ClientID == nil {
		return
	}
	value, ok := optionalAmount(d.Value)
	if !ok {
		d.Errors["project_value"] = "Enter a value of 0 or more, e.g. 15000000."
	}
	cost, ok := optionalAmount(d.Cost)
	if !ok {
		d.Errors["project_cost"] = "Enter a cost of 0 or more, e.g. 4000000."
	}
	if cost.Valid && !value.Valid && d.Errors["project_value"] == "" {
		d.Errors["project_value"] = "Enter the project value to record its cost."
	}
	in.Value, in.Cost = value, cost
	if d.BillingDate != "" {
		t, err := time.Parse("2006-01-02", d.BillingDate)
		if err != nil {
			d.Errors["billing_date"] = "Enter a valid date."
		} else {
			in.BillingDate = &t
		}
	}
}

func projectMoneyError(err error, d *projectFormData) bool {
	switch {
	case errors.Is(err, project.ErrCurrencyLocked):
		d.Errors["currency"] = "The currency cannot change after payments are recorded."
	case errors.Is(err, project.ErrValueBelowPaid):
		d.Errors["project_value"] = "The value cannot be lower than the amount already received."
	case errors.Is(err, project.ErrInternalPayments):
		d.Errors["owner"] = "A project with recorded payments must keep a client."
	default:
		return false
	}
	return true
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
	d := projectFormData{IsEdit: true, ID: p.ID, Name: p.Name, Owner: "internal", URL: deref(p.Url), Brief: p.Brief, Currency: p.Currency, BillingDate: dateValue(p.BillingDate)}
	if p.ProjectValue.Valid {
		d.Value = invoice.FormatDecimal(p.ProjectValue.Amount)
	}
	if p.ProjectCost.Valid {
		d.Cost = invoice.FormatDecimal(p.ProjectCost.Amount)
	}
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
		if !projectMoneyError(err, &d) {
			a.serverError(w, r, err)
			return
		}
		if err := a.loadProjectFormOptions(r, &d); err != nil {
			a.serverError(w, r, err)
			return
		}
		a.render(w, r, http.StatusUnprocessableEntity, "project_form", "Edit "+p.Name, "projects", d)
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
	Money     project.Money
	Payments  []sqlc.ListProjectPaymentsRow
	Today     string
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
	d := projectDetailData{Project: p, BriefHTML: a.Markdown.Render(p.Brief), Shot: shot, Today: today.Format("2006-01-02")}
	if d.Money, err = a.Projects.Money(r.Context(), p); err != nil {
		a.serverError(w, r, err)
		return
	}
	if d.Payments, err = a.Projects.Payments(r.Context(), p.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
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

func (a *App) ProjectPaymentCreate(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	back := "/projects/" + p.ID
	in := project.PaymentInput{Note: strings.TrimSpace(r.PostFormValue("note"))}
	if id := r.PostFormValue("invoice_id"); isUUID(id) {
		in.InvoiceID = &id
		if r.PostFormValue("return") == "invoice" {
			back = "/invoices/" + id
		}
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(r.PostFormValue("received_on")))
	if err != nil {
		a.redirectWithFlash(w, r, back, "Enter the date the payment was received.", true)
		return
	}
	in.ReceivedOn = date
	amount, err := money.Parse(r.PostFormValue("amount"))
	if err != nil || amount <= 0 {
		a.redirectWithFlash(w, r, back, "Enter a payment amount above 0.", true)
		return
	}
	in.Amount = amount
	err = a.Projects.RecordPayment(r.Context(), p.ID, in)
	switch {
	case errors.Is(err, project.ErrNoValue):
		a.redirectWithFlash(w, r, back, "Set a project value and a client before recording payments.", true)
	case errors.Is(err, project.ErrOverpaid):
		a.redirectWithFlash(w, r, back, "The payment is more than the remaining project value.", true)
	case errors.Is(err, project.ErrAlreadyRecorded):
		a.redirectWithFlash(w, r, back, "This invoice payment is already recorded for the project.", true)
	case err != nil:
		a.serverError(w, r, err)
	default:
		a.redirectWithFlash(w, r, back, "Payment recorded.", false)
	}
}

func (a *App) ProjectPaymentDelete(w http.ResponseWriter, r *http.Request) {
	p, ok := a.loadProject(w, r)
	if !ok {
		return
	}
	pid := r.PathValue("pid")
	if !isUUID(pid) {
		a.notFound(w, r)
		return
	}
	err := a.Projects.DeletePayment(r.Context(), p.ID, pid)
	switch {
	case errors.Is(err, project.ErrPaymentNotFound):
		a.notFound(w, r)
	case err != nil:
		a.serverError(w, r, err)
	default:
		a.redirectWithFlash(w, r, "/projects/"+p.ID, "Payment deleted.", false)
	}
}
