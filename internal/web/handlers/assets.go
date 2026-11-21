package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type assetListData struct {
	Filter        asset.Filter
	Items         []asset.ListItem
	Types         []assettype.Type
	Providers     []sqlc.ListProvidersRow
	Clients       []sqlc.ListClientOptionsRow
	Statuses      []asset.Status
	SortOptions   []struct{ Value, Label string }
	Filtered      bool
	ActiveFilters int
}

func filterFromQuery(q url.Values) asset.Filter {
	f := asset.Filter{
		TypeID:     q.Get("type"),
		Status:     q.Get("status"),
		Owner:      q.Get("owner"),
		ProviderID: q.Get("provider"),
		Query:      strings.TrimSpace(q.Get("q")),
		Sort:       q.Get("sort"),
	}
	if _, ok := asset.ParseStatus(f.Status); !ok {
		f.Status = ""
	}
	if !isUUID(f.TypeID) {
		f.TypeID = ""
	}
	if !isUUID(f.ProviderID) {
		f.ProviderID = ""
	}
	if f.Owner != asset.OwnerInternal && !isUUID(f.Owner) {
		f.Owner = ""
	}
	valid := false
	for _, s := range asset.SortOptions {
		if s.Value == f.Sort {
			valid = true
		}
	}
	if !valid {
		f.Sort = ""
	}
	return f
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

func (a *App) AssetList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	f := filterFromQuery(r.URL.Query())
	items, err := a.Assets.List(ctx, f, a.today(r))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := assetListData{Filter: f, Items: items, Statuses: asset.Statuses, SortOptions: asset.SortOptions,
		Filtered: f.TypeID != "" || f.Status != "" || f.Owner != "" || f.ProviderID != "" || f.Query != ""}
	for _, v := range []string{f.TypeID, f.Status, f.Owner, f.ProviderID, f.Sort} {
		if v != "" {
			data.ActiveFilters++
		}
	}
	if isHTMX(r) && r.Header.Get("HX-Target") == "asset-results" {
		a.fragment(w, r, http.StatusOK, "assets_list", "asset_results", data)
		return
	}
	if data.Types, err = a.AssetTypes.List(ctx); err != nil {
		a.serverError(w, r, err)
		return
	}
	if data.Providers, err = a.Providers.List(ctx); err != nil {
		a.serverError(w, r, err)
		return
	}
	if data.Clients, err = a.Clients.Options(ctx); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "assets_list", "Assets", "assets", data)
}

type assetFormData struct {
	IsEdit       bool
	ID           string
	Form         asset.Form
	Errors       map[string]string
	Types        []assettype.Type
	Providers    []sqlc.ListProvidersRow
	Clients      []sqlc.ListClientOptionsRow
	Fields       []assettype.Field
	SupportsRDAP bool
	TypeLocked   bool
	Periods      []int
	Currencies   []money.Currency
}

func (a *App) loadAssetFormOptions(r *http.Request, d *assetFormData) error {
	ctx := r.Context()
	var err error
	if d.Types, err = a.AssetTypes.List(ctx); err != nil {
		return err
	}
	if d.Providers, err = a.Providers.List(ctx); err != nil {
		return err
	}
	if d.Clients, err = a.Clients.Options(ctx); err != nil {
		return err
	}
	if d.Form.TypeID == "" && len(d.Types) > 0 {
		d.Form.TypeID = d.Types[0].ID
	}
	for _, t := range d.Types {
		if t.ID == d.Form.TypeID {
			d.Fields = t.Fields
			d.SupportsRDAP = t.SupportsRDAP
		}
	}
	d.Periods = asset.RenewalPeriods
	d.Currencies = money.Currencies
	if d.Errors == nil {
		d.Errors = map[string]string{}
	}
	return nil
}

func formFromDetail(d asset.Detail, fields []assettype.Field) asset.Form {
	f := asset.Form{
		TypeID: d.AssetTypeID, ProviderID: d.ProviderID, Owner: asset.OwnerInternal, Name: d.Name,
		ExpirationDate: d.ExpirationDate.Format("2006-01-02"), RenewalPeriod: strconv.Itoa(int(d.RenewalPeriodMonths)),
		AutoRenew: d.AutoRenew, CostPrice: d.CostPrice.String(), Currency: d.Currency, RDAPEnabled: d.RdapEnabled,
		Custom: map[string]string{}, CustomPresent: map[string]bool{},
	}
	if f.IsCustomPeriod() {
		f.RenewalCustom = f.RenewalPeriod
		f.RenewalPeriod = "custom"
	}
	if d.ClientID != nil {
		f.Owner = *d.ClientID
	}
	if d.Url != nil {
		f.URL = *d.Url
	}
	if d.OrderDate != nil {
		f.OrderDate = d.OrderDate.Format("2006-01-02")
	}
	if d.SellingPrice.Valid {
		f.SellingPrice = d.SellingPrice.Amount.String()
	}
	for _, fd := range fields {
		f.Custom[fd.Key] = d.Custom.Input(fd.Key)
	}
	return f
}

func (a *App) AssetNew(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := assetFormData{Form: asset.Form{Owner: asset.OwnerInternal, RenewalPeriod: "12", Currency: string(all.General.DefaultCurrency), RDAPEnabled: true, Custom: map[string]string{}}}
	if from := r.URL.Query().Get("from"); isUUID(from) {
		src, err := a.Assets.Get(r.Context(), from, a.today(r))
		if err == nil {
			fields, err := a.AssetTypes.Fields(r.Context(), src.AssetTypeID)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			d.Form = formFromDetail(src, fields)
			d.Form.Name = src.Name + " (copy)"
		}
	}
	if t := r.URL.Query().Get("type"); isUUID(t) {
		d.Form.TypeID = t
	}
	if o := r.URL.Query().Get("owner"); isUUID(o) {
		d.Form.Owner = o
	}
	if err := a.loadAssetFormOptions(r, &d); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "asset_form", "New asset", "assets", d)
}

func parseAssetForm(r *http.Request) asset.Form {
	f := asset.Form{
		TypeID:         r.PostFormValue("type_id"),
		ProviderID:     r.PostFormValue("provider_id"),
		ProviderNew:    r.PostFormValue("provider_new"),
		Owner:          r.PostFormValue("owner"),
		Name:           r.PostFormValue("name"),
		URL:            r.PostFormValue("url"),
		OrderDate:      r.PostFormValue("order_date"),
		ExpirationDate: r.PostFormValue("expiration_date"),
		RenewalPeriod:  r.PostFormValue("renewal_period"),
		RenewalCustom:  r.PostFormValue("renewal_custom"),
		AutoRenew:      r.PostFormValue("auto_renew") != "",
		CostPrice:      r.PostFormValue("cost_price"),
		SellingPrice:   r.PostFormValue("selling_price"),
		Currency:       r.PostFormValue("currency"),
		RDAPEnabled:    r.PostFormValue("rdap_enabled") != "",
		Custom:         map[string]string{},
		CustomPresent:  map[string]bool{},
	}
	for k, v := range r.PostForm {
		if key, ok := strings.CutPrefix(k, "cf_"); ok && len(v) > 0 {
			f.Custom[key] = v[len(v)-1]
			f.CustomPresent[key] = true
		}
	}
	return f
}

func (a *App) validateAssetForm(r *http.Request, f asset.Form) (asset.Input, assetFormData, error) {
	d := assetFormData{Form: f}
	if err := a.loadAssetFormOptions(r, &d); err != nil {
		return asset.Input{}, d, err
	}
	known := false
	for _, t := range d.Types {
		if t.ID == f.TypeID {
			known = true
		}
	}
	in, errs := f.Validate(d.Fields, d.SupportsRDAP)
	if !known {
		errs["type_id"] = "Choose an asset type."
	}
	if in.ClientID != nil {
		if _, err := a.Clients.Get(r.Context(), *in.ClientID); err != nil {
			errs["owner"] = "Choose Internal or an existing client."
		}
	}
	if in.ProviderID != "" {
		if _, err := a.Providers.Get(r.Context(), in.ProviderID); err != nil {
			errs["provider_id"] = "Choose an existing provider."
		}
	}
	d.Errors = errs
	return in, d, nil
}

func (a *App) AssetCreate(w http.ResponseWriter, r *http.Request) {
	in, d, err := a.validateAssetForm(r, parseAssetForm(r))
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(d.Errors) > 0 {
		a.render(w, r, http.StatusUnprocessableEntity, "asset_form", "New asset", "assets", d)
		return
	}
	id, err := a.Assets.Create(r.Context(), in)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+id, "Asset created.", false)
}

func (a *App) AssetEdit(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	fields, err := a.AssetTypes.Fields(r.Context(), det.AssetTypeID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d := assetFormData{IsEdit: true, ID: det.ID, Form: formFromDetail(det, fields), TypeLocked: asset.HasCustomValues(det.Custom)}
	if err := a.loadAssetFormOptions(r, &d); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "asset_form", "Edit "+det.Name, "assets", d)
}

func (a *App) AssetUpdate(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	f := parseAssetForm(r)
	locked := asset.HasCustomValues(det.Custom)
	if locked {
		f.TypeID = det.AssetTypeID
	}
	in, d, err := a.validateAssetForm(r, f)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	d.IsEdit, d.ID, d.TypeLocked = true, det.ID, locked
	if len(d.Errors) == 0 {
		err = a.Assets.Update(r.Context(), det.ID, in)
		if errors.Is(err, asset.ErrTypeLocked) {
			d.Errors["type_id"] = "The type cannot change while custom field values exist."
		} else if err != nil {
			a.serverError(w, r, err)
			return
		}
	}
	if len(d.Errors) > 0 {
		a.render(w, r, http.StatusUnprocessableEntity, "asset_form", "Edit "+det.Name, "assets", d)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID, "Asset saved.", false)
}

func (a *App) AssetFields(w http.ResponseWriter, r *http.Request) {
	typeID := r.URL.Query().Get("type_id")
	d := assetFormData{Form: asset.Form{TypeID: typeID, RDAPEnabled: true, Custom: map[string]string{}}}
	if err := a.loadAssetFormOptions(r, &d); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.fragment(w, r, http.StatusOK, "asset_form", "custom_fields", d)
}

func (a *App) loadAsset(w http.ResponseWriter, r *http.Request) (asset.Detail, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return asset.Detail{}, false
	}
	d, err := a.Assets.Get(r.Context(), id, a.today(r))
	if errors.Is(err, asset.ErrNotFound) {
		a.notFound(w, r)
		return d, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return d, false
	}
	return d, true
}

func (a *App) AssetCancel(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	if err := a.Assets.Cancel(r.Context(), det.ID, a.Now()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID, "Asset cancelled. Reminders and RDAP sync are stopped.", false)
}

func (a *App) AssetRestore(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	if err := a.Assets.Restore(r.Context(), det.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID, "Asset restored.", false)
}

func (a *App) AssetDelete(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	if r.PostFormValue("confirm_name") != det.Name {
		a.redirectWithFlash(w, r, "/assets/"+det.ID, "Type the asset name exactly to delete it.", true)
		return
	}
	failed, err := a.Assets.Delete(r.Context(), det.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if len(failed) > 0 {
		a.Logger.Error("delete asset files", "asset_id", det.ID, "failed_files", len(failed))
	}
	a.redirectWithFlash(w, r, "/assets", "Asset deleted.", false)
}

func (a *App) AssetNotes(w http.ResponseWriter, r *http.Request) {
	det, ok := a.loadAsset(w, r)
	if !ok {
		return
	}
	notes := strings.ReplaceAll(r.PostFormValue("notes"), "\r\n", "\n")
	if err := a.Assets.UpdateNotes(r.Context(), det.ID, notes); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/assets/"+det.ID+"?tab=notes", "Notes saved.", false)
}

func (a *App) MarkdownPreview(w http.ResponseWriter, r *http.Request) {
	src := r.PostFormValue("notes")
	if src == "" {
		src = r.PostFormValue("brief")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := a.Markdown.Render(src)
	if strings.TrimSpace(string(html)) == "" {
		html = "<p class=\"text-slate-500\">Nothing to preview.</p>"
	}
	_, _ = w.Write([]byte(html))
}

type searchData struct {
	Query   string
	Results any
	Empty   bool
}

func (a *App) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res, err := a.Assets.Search(r.Context(), q)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	p := a.page(w, r, "Search", "", searchData{Query: q, Results: res, Empty: q != "" && len(res.Assets)+len(res.Clients)+len(res.Projects) == 0})
	p.SearchQuery = q
	if err := a.Render.Page(w, http.StatusOK, "search", p); err != nil {
		a.Logger.Error("render search", "error", err)
	}
}
