package handlers

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/invoice"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/settings"
	"github.com/suwahadi/project-ops-manager/internal/web/static"
)

type PDFPrinter interface {
	PrintPDF(ctx context.Context, html string) ([]byte, error)
}

type invoiceRow struct {
	invoice.Summary
	Overdue bool
}

type invoiceListData struct {
	Filter        invoice.Filter
	Items         []invoiceRow
	Totals        []invoice.CurrencyTotals
	Clients       []sqlc.ListClientOptionsRow
	StatusOptions []invoice.Option
	SortOptions   []invoice.Option
}

func (a *App) InvoiceList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Invoices.List(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	today := a.today(r)
	f := invoice.ParseFilter(r.URL.Query(), isUUID)
	filtered := invoice.Apply(rows, f, today)
	data := invoiceListData{Filter: f, Totals: invoice.Summarize(filtered, today), StatusOptions: invoice.StatusOptions, SortOptions: invoice.SortOptions}
	for _, s := range filtered {
		data.Items = append(data.Items, invoiceRow{Summary: s, Overdue: invoice.Overdue(s.Status, s.DueDate, today)})
	}
	if isHTMX(r) && r.Header.Get("HX-Target") == "invoice-results" {
		a.fragment(w, r, http.StatusOK, "invoices_list", "invoice_results", data)
		return
	}
	if data.Clients, err = a.Clients.Options(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "invoices_list", "Invoices", "invoices", data)
}

type invoiceFormData struct {
	IsEdit     bool
	ID         string
	Form       invoice.Form
	Errors     map[string]string
	Clients    []sqlc.ListClientOptionsRow
	Currencies []money.Currency
	Prefix     string
}

func (d invoiceFormData) ItemError(field string, i int) string {
	return d.Errors[invoice.ItemKey(field, i)]
}

func (a *App) renderInvoiceForm(w http.ResponseWriter, r *http.Request, status int, d invoiceFormData) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if d.Clients, err = a.Clients.Options(r.Context()); err != nil {
		a.serverError(w, r, err)
		return
	}
	if d.Errors == nil {
		d.Errors = map[string]string{}
	}
	if len(d.Form.Items) == 0 {
		d.Form.Items = []invoice.ItemForm{{Quantity: "1", Tax: "0"}}
	}
	d.Currencies = money.Currencies
	d.Prefix = all.Invoice.NumberPrefix
	title := "New invoice"
	if d.IsEdit {
		title = "Edit " + d.Form.Number
	}
	a.render(w, r, status, "invoice_form", title, "invoices", d)
}

func renewalSource(d asset.Detail) invoice.RenewalSource {
	return invoice.RenewalSource{
		Name: d.Name, TypeName: d.TypeName, Expiration: d.ExpirationDate, Months: int(d.RenewalPeriodMonths),
		Cost: d.CostPrice, Selling: d.SellingPrice, Currency: money.Currency(d.Currency),
	}
}

func (a *App) clientInvoiceItems(ctx context.Context, clientID string, preferred money.Currency) (money.Currency, []invoice.ItemForm, error) {
	rows, err := a.Clients.Assets(ctx, clientID)
	if err != nil {
		return preferred, nil, err
	}
	sources := make([]invoice.RenewalSource, len(rows))
	for i, r := range rows {
		sources[i] = invoice.RenewalSource{
			Name: r.Name, TypeName: r.TypeName, Expiration: r.ExpirationDate, Months: int(r.RenewalPeriodMonths),
			Cost: r.CostPrice, Selling: r.SellingPrice, Currency: money.Currency(r.Currency), Cancelled: r.CancelledAt != nil,
		}
	}
	cur, items := invoice.ClientItems(sources, preferred)
	return cur, items, nil
}

func (a *App) InvoiceNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := a.Settings.Load(ctx)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	number, err := a.Invoices.UniqueNumber(ctx, all.Invoice.NumberPrefix)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	today := a.today(r)
	f := invoice.Form{
		Number: number, Currency: string(all.General.DefaultCurrency), InvoiceDate: today.Format("2006-01-02"),
		Status: invoice.StatusUnpaid, Note: all.Invoice.DefaultNote,
	}
	if id := r.URL.Query().Get("asset"); isUUID(id) {
		d, err := a.Assets.Get(ctx, id, today)
		if err == nil {
			f.AssetID = d.ID
			f.Currency = d.Currency
			f.Items = []invoice.ItemForm{invoice.RenewalItem(renewalSource(d))}
			if d.ClientID != nil {
				f.ClientID = *d.ClientID
				f.BillTo = deref(d.ClientName)
			}
		}
	}
	if id := r.URL.Query().Get("client"); isUUID(id) && f.ClientID == "" {
		if c, err := a.Clients.Get(ctx, id); err == nil {
			f.ClientID = c.ID
			f.BillTo = c.Name
			cur, items, err := a.clientInvoiceItems(ctx, c.ID, all.General.DefaultCurrency)
			if err != nil {
				a.serverError(w, r, err)
				return
			}
			f.Currency = string(cur)
			f.Items = items
		}
	}
	a.renderInvoiceForm(w, r, http.StatusOK, invoiceFormData{Form: f})
}

func invoiceFormFrom(r *http.Request) invoice.Form {
	_ = r.ParseForm()
	v := r.PostForm
	f := invoice.Form{
		Number: v.Get("number"), ClientID: v.Get("client_id"), AssetID: v.Get("asset_id"), BillTo: v.Get("bill_to"),
		Currency: v.Get("currency"), InvoiceDate: v.Get("invoice_date"), DueDate: v.Get("due_date"),
		Status: v.Get("status"), PaidDate: v.Get("paid_date"), Note: v.Get("note"),
	}
	if !isUUID(f.AssetID) {
		f.AssetID = ""
	}
	titles := v["item_title"]
	for i := range titles {
		at := func(key string) string {
			if vals := v[key]; i < len(vals) {
				return vals[i]
			}
			return ""
		}
		f.Items = append(f.Items, invoice.ItemForm{
			Title: titles[i], Description: at("item_description"), Quantity: at("item_quantity"), Rate: at("item_rate"), Tax: at("item_tax"),
		})
	}
	if len(f.Items) > invoice.MaxItems+1 {
		f.Items = f.Items[:invoice.MaxItems+1]
	}
	return f
}

func (a *App) clientNameLookup(ctx context.Context) func(string) (string, bool) {
	return func(id string) (string, bool) {
		if !isUUID(id) {
			return "", false
		}
		c, err := a.Clients.Get(ctx, id)
		if err != nil {
			return "", false
		}
		return c.Name, true
	}
}

func (a *App) InvoiceCreate(w http.ResponseWriter, r *http.Request) {
	f := invoiceFormFrom(r)
	in, errs := f.Validate(a.clientNameLookup(r.Context()))
	if len(errs) == 0 {
		id, err := a.Invoices.Create(r.Context(), in)
		switch {
		case errors.Is(err, invoice.ErrNumberTaken):
			errs["number"] = "This invoice number is already used."
		case err != nil:
			a.serverError(w, r, err)
			return
		default:
			a.redirectWithFlash(w, r, "/invoices/"+id, "Invoice created.", false)
			return
		}
	}
	a.renderInvoiceForm(w, r, http.StatusUnprocessableEntity, invoiceFormData{Form: f, Errors: errs})
}

func (a *App) loadInvoice(w http.ResponseWriter, r *http.Request) (invoice.Invoice, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		a.notFound(w, r)
		return invoice.Invoice{}, false
	}
	inv, err := a.Invoices.Get(r.Context(), id)
	if errors.Is(err, invoice.ErrNotFound) {
		a.notFound(w, r)
		return inv, false
	}
	if err != nil {
		a.serverError(w, r, err)
		return inv, false
	}
	return inv, true
}

func dateValue(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func formFromInvoice(inv invoice.Invoice) invoice.Form {
	f := invoice.Form{
		Number: inv.Number, ClientID: deref(inv.ClientID), AssetID: deref(inv.AssetID), BillTo: inv.BillTo, Currency: inv.Currency,
		InvoiceDate: inv.InvoiceDate.Format("2006-01-02"), DueDate: dateValue(inv.DueDate), Status: inv.Status,
		PaidDate: dateValue(inv.PaidDate), Note: inv.Note,
	}
	for _, it := range inv.Items {
		f.Items = append(f.Items, invoice.ItemForm{
			Title: it.Title, Description: it.Description, Quantity: invoice.FormatDecimal(it.Quantity),
			Rate: invoice.FormatDecimal(it.Rate), Tax: invoice.FormatDecimal(it.TaxPercent),
		})
	}
	return f
}

func (a *App) InvoiceEdit(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	a.renderInvoiceForm(w, r, http.StatusOK, invoiceFormData{IsEdit: true, ID: inv.ID, Form: formFromInvoice(inv)})
}

func (a *App) InvoiceUpdate(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	f := invoiceFormFrom(r)
	f.AssetID = deref(inv.AssetID)
	in, errs := f.Validate(a.clientNameLookup(r.Context()))
	if len(errs) == 0 {
		err := a.Invoices.Update(r.Context(), inv.ID, in)
		switch {
		case errors.Is(err, invoice.ErrNumberTaken):
			errs["number"] = "This invoice number is already used."
		case err != nil:
			a.serverError(w, r, err)
			return
		default:
			a.redirectWithFlash(w, r, "/invoices/"+inv.ID, "Invoice saved.", false)
			return
		}
	}
	a.renderInvoiceForm(w, r, http.StatusUnprocessableEntity, invoiceFormData{IsEdit: true, ID: inv.ID, Form: f, Errors: errs})
}

type invoiceSheet struct {
	Invoice invoice.Invoice
	Company settings.Invoice
	Overdue bool
}

func (s invoiceSheet) Line(it sqlc.InvoiceItem) invoice.Line { return invoice.ItemLine(it) }

type invoiceDetailData struct {
	Sheet invoiceSheet
	Today string
}

func (a *App) invoiceSheet(r *http.Request, inv invoice.Invoice) (invoiceSheet, error) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		return invoiceSheet{}, err
	}
	return invoiceSheet{Invoice: inv, Company: all.Invoice, Overdue: invoice.Overdue(inv.Status, inv.DueDate, a.today(r))}, nil
}

func (a *App) InvoiceShow(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	sheet, err := a.invoiceSheet(r, inv)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "invoice_detail", inv.Number, "invoices", invoiceDetailData{Sheet: sheet, Today: a.today(r).Format("2006-01-02")})
}

type invoiceDocument struct {
	Title string
	CSS   template.CSS
	Sheet invoiceSheet
}

func (a *App) InvoicePDF(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	sheet, err := a.invoiceSheet(r, inv)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	css, err := static.FS.ReadFile("css/app.css")
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	html, err := a.Render.HTML("invoice_pdf", "document", invoiceDocument{Title: inv.Number, CSS: template.CSS(css), Sheet: sheet})
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	pdf, err := a.PDF.PrintPDF(r.Context(), string(html))
	if err != nil {
		a.Logger.Warn("invoice pdf failed", "invoice", inv.ID, "error", err.Error())
		a.redirectWithFlash(w, r, "/invoices/"+inv.ID, "Could not create the PDF: "+err.Error(), true)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+inv.Number+`.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(pdf)
}

func (a *App) InvoiceMarkPaid(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	paid, err := time.Parse("2006-01-02", strings.TrimSpace(r.PostFormValue("paid_date")))
	if err != nil {
		a.redirectWithFlash(w, r, "/invoices/"+inv.ID, "Enter the payment date.", true)
		return
	}
	if err := a.Invoices.MarkPaid(r.Context(), inv.ID, paid); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/invoices/"+inv.ID, "Invoice marked as paid.", false)
}

func (a *App) InvoiceMarkUnpaid(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	if err := a.Invoices.MarkUnpaid(r.Context(), inv.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/invoices/"+inv.ID, "Invoice marked as unpaid.", false)
}

func (a *App) InvoiceDuplicate(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	number, err := a.Invoices.UniqueNumber(r.Context(), all.Invoice.NumberPrefix)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	f := formFromInvoice(inv)
	f.Number = number
	f.InvoiceDate = a.today(r).Format("2006-01-02")
	f.DueDate = ""
	f.Status = invoice.StatusUnpaid
	f.PaidDate = ""
	f.AssetID = ""
	a.renderInvoiceForm(w, r, http.StatusOK, invoiceFormData{Form: f})
}

func (a *App) InvoiceDelete(w http.ResponseWriter, r *http.Request) {
	inv, ok := a.loadInvoice(w, r)
	if !ok {
		return
	}
	if err := a.Invoices.Delete(r.Context(), inv.ID); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/invoices", "Invoice deleted.", false)
}

type invoiceSettingsForm struct {
	Settings settings.Invoice
	Errors   FieldErrors
}

func (a *App) SettingsInvoice(w http.ResponseWriter, r *http.Request) {
	all, err := a.Settings.Load(r.Context())
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "settings_invoice", "Settings", "settings", shell("invoice", invoiceSettingsForm{Settings: all.Invoice, Errors: FieldErrors{}}))
}

func (a *App) SettingsInvoiceSave(w http.ResponseWriter, r *http.Request) {
	f := invoiceSettingsForm{
		Settings: settings.Invoice{
			CompanyName:  strings.TrimSpace(r.PostFormValue("company_name")),
			CompanyURL:   strings.TrimSpace(r.PostFormValue("company_url")),
			NumberPrefix: invoice.NormalizeNumber(r.PostFormValue("number_prefix")),
			DefaultNote:  strings.TrimRight(r.PostFormValue("default_note"), " \t\r\n"),
		},
		Errors: FieldErrors{},
	}
	if f.Settings.CompanyName == "" {
		f.Errors.Add("company_name", "Company name is required.")
	}
	if !invoice.ValidPrefix(f.Settings.NumberPrefix) {
		f.Errors.Add("number_prefix", "Use letters, digits, dots, dashes or underscores, up to 32 characters.")
	}
	if f.Errors.Any() {
		a.render(w, r, http.StatusUnprocessableEntity, "settings_invoice", "Settings", "settings", shell("invoice", f))
		return
	}
	if err := a.Settings.SaveInvoice(r.Context(), f.Settings); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.redirectWithFlash(w, r, "/settings/invoice", "Invoice settings saved.", false)
}
