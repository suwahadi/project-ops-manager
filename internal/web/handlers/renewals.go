package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type renewForm struct {
	NewExpiration string
	Cost          string
	Selling       string
	PaidOn        string
	Errors        map[string]string
	Internal      bool
	Currency      string
}

func (a *App) defaultRenewForm(det asset.Detail, today time.Time) renewForm {
	f := renewForm{
		NewExpiration: asset.DefaultNewExpiration(det.ExpirationDate, int(det.RenewalPeriodMonths)).Format("2006-01-02"),
		Cost:          det.CostPrice.String(),
		PaidOn:        today.Format("2006-01-02"),
		Errors:        map[string]string{},
		Internal:      det.ClientID == nil,
		Currency:      det.Currency,
	}
	if det.SellingPrice.Valid {
		f.Selling = det.SellingPrice.Amount.String()
	}
	return f
}

func (a *App) AssetRenew(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	uploads, uploadErr := readUploads(r, "documents")
	f := renewForm{
		NewExpiration: strings.TrimSpace(r.FormValue("new_expiration")),
		Cost:          r.FormValue("cost_price"),
		Selling:       r.FormValue("selling_price"),
		PaidOn:        strings.TrimSpace(r.FormValue("paid_on")),
		Errors:        map[string]string{},
		Internal:      det.ClientID == nil,
		Currency:      det.Currency,
	}
	var in asset.RenewInput
	var err error
	if in.NewExpiration, err = time.Parse("2006-01-02", f.NewExpiration); err != nil {
		f.Errors["new_expiration"] = "Enter the new expiration date."
	} else if !in.NewExpiration.After(det.ExpirationDate) {
		f.Errors["new_expiration"] = "The new expiration date must be after " + det.ExpirationDate.Format("02 Jan 2006") + "."
	}
	if in.PaidOn, err = time.Parse("2006-01-02", f.PaidOn); err != nil {
		f.Errors["paid_on"] = "Enter the date you paid."
	}
	if in.Cost, err = money.Parse(f.Cost); err != nil || in.Cost < 0 {
		f.Errors["cost_price"] = "Enter a cost price of 0 or more."
	}
	if strings.TrimSpace(f.Selling) != "" {
		sell, err := money.Parse(f.Selling)
		switch {
		case err != nil || sell < 0:
			f.Errors["selling_price"] = "Enter a selling price of 0 or more."
		case f.Internal:
			f.Errors["selling_price"] = "Internal assets have no selling price."
		}
		in.Selling = money.Some(sell)
	}
	if uploadErr != nil {
		f.Errors["documents"] = uploadErr.Error()
	}
	if len(f.Errors) == 0 {
		prepared, err := a.prepareUploads(uploads)
		if err != nil {
			f.Errors["documents"] = err.Error()
		} else {
			if _, err := a.Assets.Renew(r.Context(), det.ID, in, prepared); err != nil {
				a.Documents.RemovePrepared(prepared)
				if errors.Is(err, asset.ErrNotFound) {
					a.notFound(w, r)
					return
				}
				a.serverError(w, r, err)
				return
			}
			a.redirectWithFlash(w, r, "/assets/"+det.ID+"?tab=renewals", "Renewal saved. New expiration date: "+in.NewExpiration.Format("02 Jan 2006")+".", false)
			return
		}
	}
	if isHTMX(r) {
		a.fragment(w, r, http.StatusUnprocessableEntity, "asset_detail", "renew_form", renewDialogData{Asset: det, Form: f})
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID, firstOf(f.Errors), true)
}

type renewDialogData struct {
	Asset asset.Detail
	Form  renewForm
}

func firstOf(m map[string]string) string {
	for _, k := range []string{"new_expiration", "paid_on", "cost_price", "selling_price", "documents"} {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return "Please check the form."
}
