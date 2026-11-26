package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/finance"
)

type financeData struct {
	View       string
	MonthValue string
	Year       int
	Years      []int
	Report     finance.Report
	Projection []finance.MonthRow
	PrevHref   string
	NextHref   string
}

func (a *App) FinancePage(w http.ResponseWriter, r *http.Request) {
	today := a.today(r)
	q := r.URL.Query()
	view := q.Get("view")
	if view != "year" {
		view = "month"
	}
	var p finance.Period
	data := financeData{View: view}
	if view == "year" {
		y, err := strconv.Atoi(q.Get("year"))
		if err != nil || y < 2000 || y > 2100 {
			y = today.Year()
		}
		p = finance.YearPeriod(y)
		data.PrevHref = "/finance?view=year&year=" + strconv.Itoa(y-1)
		data.NextHref = "/finance?view=year&year=" + strconv.Itoa(y+1)
	} else {
		m, err := time.Parse("2006-01", q.Get("month"))
		if err != nil || m.Year() < 2000 || m.Year() > 2100 {
			m = time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		}
		p = finance.MonthPeriod(m.Year(), m.Month())
		data.PrevHref = "/finance?view=month&month=" + m.AddDate(0, -1, 0).Format("2006-01")
		data.NextHref = "/finance?view=month&month=" + m.AddDate(0, 1, 0).Format("2006-01")
	}
	data.MonthValue = p.Start.Format("2006-01")
	data.Year = p.Start.Year()
	for y := today.Year() - 5; y <= today.Year()+5; y++ {
		data.Years = append(data.Years, y)
	}
	var err error
	if data.Report, err = a.Finance.Report(r.Context(), p, today); err != nil {
		a.serverError(w, r, err)
		return
	}
	if data.Projection, err = a.Finance.Projection(r.Context(), today); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "finance", "Finance", "finance", data)
}
