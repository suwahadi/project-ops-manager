package handlers

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/rdap"
)

type banner struct {
	Message string
	Href    string
	Link    string
}

type dashboardData struct {
	Banners []banner
}

func (a *App) dashboardBanners(r *http.Request) ([]banner, error) {
	var out []banner
	last, err := a.Queries.LatestFinishedReminderLog(r.Context())
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, err
	case last.Status == "failed":
		out = append(out, banner{Message: "The last reminder delivery failed.", Href: "/assets/" + last.AssetID + "?tab=reminders", Link: "View delivery log"})
	}
	run, err := a.Queries.LatestJobRun(r.Context(), rdap.JobName)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, err
	case run.Status == "failed":
		msg := "The last RDAP sync run failed."
		if run.Error != nil {
			msg = "The last RDAP sync run failed: " + *run.Error + "."
		}
		out = append(out, banner{Message: msg, Href: "/rdap", Link: "View RDAP log"})
	}
	return out, nil
}

func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	banners, err := a.dashboardBanners(r)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "dashboard", "Dashboard", "dashboard", dashboardData{Banners: banners})
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	a.notFound(w, r)
}
