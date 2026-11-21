package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/finance"
)

type clientFormData struct {
	IsEdit bool
	ID     string
	Input  client.Input
	Errors map[string]string
}

func clientInputFrom(r *http.Request) (client.Input, map[string]string) {
	in := client.Input{
		Name:          strings.TrimSpace(r.PostFormValue("name")),
		ContactPerson: r.PostFormValue("contact_person"),
		Email:         strings.TrimSpace(r.PostFormValue("email")),
		Phone:         r.PostFormValue("phone"),
		Notes:         r.PostFormValue("notes"),
	}
	errs := map[string]string{}
	if in.Name == "" {
		errs["name"] = "Name is required."
	}
	if in.Email != "" {
		if _, err := mail.ParseAddress(in.Email); err != nil {
			errs["email"] = "Enter a valid email address."
		}
	}
	return in, errs
}

func (a *App) ClientList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Clients.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "clients_list", "Clients", "clients", rows)
}

func (a *App) ClientNew(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "client_form", "New client", "clients", clientFormData{Errors: map[string]string{}})
}

func (a *App) ClientCreate(w http.ResponseWriter, r *http.Request) {
	in, errs := clientInputFrom(r)
	if len(errs) > 0 {
		a.render(w, r, http.StatusUnprocessableEntity, "client_form", "New client", "clients", clientFormData{Input: in, Errors: errs})
		return
	}
	id, err := a.Clients.Create(r.Context(), in)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/clients/"+id, "Client created.", false)
}

func (a *App) loadClient(w http.ResponseWriter, r *http.Request) (sqlc.Client, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return sqlc.Client{}, false
	}
	c, err := a.Clients.Get(r.Context(), id)
	if errors.Is(err, client.ErrNotFound) {
		a.notFound(w, r)
		return c, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return c, false
	}
	return c, true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (a *App) ClientEdit(w http.ResponseWriter, r *http.Request) {
	c, ok := a.loadClient(w, r)
	if !ok {
		return
	}
	in := client.Input{Name: c.Name, ContactPerson: deref(c.ContactPerson), Email: deref(c.Email), Phone: deref(c.Phone), Notes: deref(c.Notes)}
	a.render(w, r, http.StatusOK, "client_form", "Edit "+c.Name, "clients", clientFormData{IsEdit: true, ID: c.ID, Input: in, Errors: map[string]string{}})
}

func (a *App) ClientUpdate(w http.ResponseWriter, r *http.Request) {
	c, ok := a.loadClient(w, r)
	if !ok {
		return
	}
	in, errs := clientInputFrom(r)
	if len(errs) > 0 {
		a.render(w, r, http.StatusUnprocessableEntity, "client_form", "Edit "+c.Name, "clients", clientFormData{IsEdit: true, ID: c.ID, Input: in, Errors: errs})
		return
	}
	if err := a.Clients.Update(r.Context(), c.ID, in); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/clients/"+c.ID, "Client saved.", false)
}

func (a *App) ClientDelete(w http.ResponseWriter, r *http.Request) {
	c, ok := a.loadClient(w, r)
	if !ok {
		return
	}
	err := a.Clients.Delete(r.Context(), c.ID)
	if errors.Is(err, client.ErrInUse) {
		a.redirectWithFlash(w, r, "/clients/"+c.ID, "This client still has assets or projects and cannot be deleted.", true)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/clients", "Client deleted.", false)
}

type clientAssetView struct {
	sqlc.ListAssetsByClientRow
	Status    asset.Status
	Countdown string
}

type clientDetailData struct {
	Client   sqlc.Client
	Assets   []clientAssetView
	Projects []sqlc.ListProjectsByClientRow
	Totals   []finance.AnnualTotal
}

func (a *App) ClientShow(w http.ResponseWriter, r *http.Request) {
	c, ok := a.loadClient(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	rows, err := a.Clients.Assets(ctx, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	projects, err := a.Clients.Projects(ctx, c.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	today := a.today(r)
	data := clientDetailData{Client: c, Projects: projects}
	var recurring []finance.Recurring
	for _, row := range rows {
		cancelled := row.CancelledAt != nil
		data.Assets = append(data.Assets, clientAssetView{
			ListAssetsByClientRow: row,
			Status:                asset.ComputeStatus(cancelled, row.ExpirationDate, today, row.ReminderOffsets),
			Countdown:             asset.Countdown(row.ExpirationDate, today),
		})
		if !cancelled {
			recurring = append(recurring, finance.Recurring{Currency: row.Currency, Cost: row.CostPrice, Selling: row.SellingPrice, PeriodMonths: int(row.RenewalPeriodMonths)})
		}
	}
	data.Totals = finance.Annualize(recurring)
	a.render(w, r, http.StatusOK, "client_detail", c.Name, "clients", data)
}
