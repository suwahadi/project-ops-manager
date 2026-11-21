package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/provider"
)

type providersData struct {
	Providers []sqlc.ListProvidersRow
	Form      providerFormData
}

type providerFormData struct {
	IsEdit bool
	ID     string
	Input  provider.Input
	Errors map[string]string
}

func providerInputFrom(r *http.Request) (provider.Input, map[string]string) {
	in := provider.Input{
		Name:     strings.TrimSpace(r.PostFormValue("name")),
		Website:  strings.TrimSpace(r.PostFormValue("website")),
		LoginURL: strings.TrimSpace(r.PostFormValue("login_url")),
		Notes:    r.PostFormValue("notes"),
	}
	errs := map[string]string{}
	if in.Name == "" {
		errs["name"] = "Name is required."
	}
	if in.Website != "" && !assettype.ValidURL(in.Website) {
		errs["website"] = "Enter a valid http or https URL."
	}
	if in.LoginURL != "" && !assettype.ValidURL(in.LoginURL) {
		errs["login_url"] = "Enter a valid http or https URL."
	}
	return in, errs
}

func (a *App) renderProviders(w http.ResponseWriter, r *http.Request, status int, form providerFormData) {
	rows, err := a.Providers.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if form.Errors == nil {
		form.Errors = map[string]string{}
	}
	a.render(w, r, status, "settings_providers", "Settings", "settings", shell("providers", providersData{Providers: rows, Form: form}))
}

func (a *App) ProviderList(w http.ResponseWriter, r *http.Request) {
	a.renderProviders(w, r, http.StatusOK, providerFormData{})
}

func (a *App) ProviderCreate(w http.ResponseWriter, r *http.Request) {
	in, errs := providerInputFrom(r)
	if len(errs) == 0 {
		_, err := a.Providers.Create(r.Context(), in)
		if errors.Is(err, provider.ErrDuplicate) {
			errs["name"] = "A provider with this name already exists."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		a.renderProviders(w, r, http.StatusUnprocessableEntity, providerFormData{Input: in, Errors: errs})
		return
	}
	a.redirectWithFlash(w, r, "/settings/providers", "Provider added.", false)
}

func (a *App) ProviderEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := a.Providers.Get(r.Context(), id)
	if err != nil || !isUUID(id) {
		a.notFound(w, r)
		return
	}
	a.renderProviders(w, r, http.StatusOK, providerFormData{IsEdit: true, ID: p.ID, Input: provider.Input{Name: p.Name, Website: deref(p.Website), LoginURL: deref(p.LoginUrl), Notes: deref(p.Notes)}})
}

func (a *App) ProviderUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.Providers.Get(r.Context(), id); err != nil || !isUUID(id) {
		a.notFound(w, r)
		return
	}
	in, errs := providerInputFrom(r)
	if len(errs) == 0 {
		err := a.Providers.Update(r.Context(), id, in)
		if errors.Is(err, provider.ErrDuplicate) {
			errs["name"] = "A provider with this name already exists."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if len(errs) > 0 {
		a.renderProviders(w, r, http.StatusUnprocessableEntity, providerFormData{IsEdit: true, ID: id, Input: in, Errors: errs})
		return
	}
	a.redirectWithFlash(w, r, "/settings/providers", "Provider saved.", false)
}

func (a *App) ProviderDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return
	}
	err := a.Providers.Delete(r.Context(), id)
	if errors.Is(err, provider.ErrInUse) {
		a.redirectWithFlash(w, r, "/settings/providers", "This provider still has assets and cannot be deleted.", true)
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/providers", "Provider deleted.", false)
}
