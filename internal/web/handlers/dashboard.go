package handlers

import "net/http"

func (a *App) Dashboard(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "dashboard", "Dashboard", "dashboard", nil)
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	a.notFound(w, r)
}
