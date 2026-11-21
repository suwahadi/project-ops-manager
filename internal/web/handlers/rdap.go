package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/rdap"
)

func (a *App) RDAPCheck(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	out, err := a.RDAP.CheckAsset(ctx, det.ID)
	back := "/assets/" + det.ID
	switch {
	case errors.Is(err, rdap.ErrNotEligible):
		a.redirectWithFlash(w, r, back, "RDAP sync is off for this asset.", true)
		return
	case err != nil:
		a.serverError(w, r, err)
		return
	}
	switch out.Result {
	case rdap.ResultUpdated:
		a.redirectWithFlash(w, r, back, "Expiration date updated from RDAP: "+out.NewExpiration.Format("02 Jan 2006")+".", false)
	case rdap.ResultUnchanged:
		a.redirectWithFlash(w, r, back, "RDAP confirms the expiration date.", false)
	case rdap.ResultUnsupported:
		a.redirectWithFlash(w, r, back, "RDAP is not available for this domain; the date was not changed.", true)
	default:
		a.redirectWithFlash(w, r, back, "RDAP check failed: "+out.Error, true)
	}
}

type rdapLogData struct {
	Runs     []sqlc.JobRun
	Domains  []sqlc.ListRDAPStatusRow
	Location *time.Location
}

func (a *App) RDAPLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runs, err := a.Queries.ListJobRuns(ctx, sqlc.ListJobRunsParams{Job: rdap.JobName, Limit: 20})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	domains, err := a.Queries.ListRDAPStatus(ctx)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "rdap_log", "RDAP sync", "assets", rdapLogData{Runs: runs, Domains: domains, Location: a.location(r)})
}
