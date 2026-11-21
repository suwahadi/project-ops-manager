package handlers

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/finance"
	"github.com/suwahadi/project-ops-manager/internal/rdap"
)

const upcomingWindowDays = 30

type banner struct {
	Message string
	Href    string
	Link    string
}

type upcomingRow struct {
	sqlc.ListUpcomingRenewalsRow
	Status    asset.Status
	Countdown string
}

type statusCount struct {
	Status asset.Status
	Count  int64
}

type financeSummary struct {
	Label     string
	Actual    []finance.Line
	Projected []finance.Line
	Href      string
}

type dashboardData struct {
	Banners     []banner
	Upcoming    []upcomingRow
	Statuses    []statusCount
	Types       []sqlc.CountAssetsPerTypeRow
	Month       financeSummary
	Year        financeSummary
	TotalAssets int64
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
	ctx := r.Context()
	today := a.today(r)
	var d dashboardData
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		d.Banners, err = a.dashboardBanners(r.WithContext(gctx))
		return err
	})
	g.Go(func() error {
		rows, err := a.Queries.ListUpcomingRenewals(gctx, today.AddDate(0, 0, upcomingWindowDays))
		if err != nil {
			return err
		}
		for _, row := range rows {
			d.Upcoming = append(d.Upcoming, upcomingRow{ListUpcomingRenewalsRow: row,
				Status: asset.ComputeStatus(false, row.ExpirationDate, today, row.ReminderOffsets), Countdown: asset.Countdown(row.ExpirationDate, today)})
		}
		return nil
	})
	g.Go(func() error {
		c, err := a.Queries.CountAssetsByStatus(gctx, today)
		if err != nil {
			return err
		}
		d.Statuses = []statusCount{{asset.StatusActive, c.Active}, {asset.StatusExpiringSoon, c.Expiring}, {asset.StatusExpired, c.Expired}, {asset.StatusCancelled, c.Cancelled}}
		d.TotalAssets = c.Active + c.Expiring + c.Expired + c.Cancelled
		return nil
	})
	g.Go(func() error {
		var err error
		d.Types, err = a.Queries.CountAssetsPerType(gctx)
		return err
	})
	g.Go(func() error {
		p := finance.MonthPeriod(today.Year(), today.Month())
		act, proj, err := a.Finance.Summary(gctx, p)
		d.Month = financeSummary{Label: "This month · " + p.Start.Format("January 2006"), Actual: act, Projected: proj, Href: "/finance?view=month&month=" + p.Start.Format("2006-01")}
		return err
	})
	g.Go(func() error {
		p := finance.YearPeriod(today.Year())
		act, proj, err := a.Finance.Summary(gctx, p)
		d.Year = financeSummary{Label: "This year · " + p.Start.Format("2006"), Actual: act, Projected: proj, Href: "/finance?view=year&year=" + p.Start.Format("2006")}
		return err
	})
	if err := g.Wait(); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "dashboard", "Dashboard", "dashboard", d)
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	a.notFound(w, r)
}
