package invoice

import (
	"strconv"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

const (
	StatusUnpaid = "unpaid"
	StatusPaid   = "paid"
	MaxItems     = 100
	dateLayout   = "2006-01-02"
)

type ItemForm struct {
	ProjectID   string
	Title       string
	Description string
	Quantity    string
	Rate        string
	Tax         string
}

type Form struct {
	Number      string
	ClientID    string
	AssetID     string
	BillTo      string
	Currency    string
	InvoiceDate string
	DueDate     string
	Status      string
	PaidDate    string
	Note        string
	Items       []ItemForm
}

type Item struct {
	ProjectID   *string
	Title       string
	Description string
	Quantity    money.Amount
	Rate        money.Amount
	TaxPercent  money.Amount
}

func (i Item) Line() Line {
	return Line{Quantity: i.Quantity, Rate: i.Rate, TaxPercent: i.TaxPercent}
}

type Input struct {
	Number      string
	ClientID    *string
	AssetID     *string
	BillTo      string
	Currency    money.Currency
	InvoiceDate time.Time
	DueDate     *time.Time
	Paid        bool
	PaidDate    *time.Time
	Note        string
	Items       []Item
}

func (in Input) Lines() []Line {
	lines := make([]Line, len(in.Items))
	for i, it := range in.Items {
		lines[i] = it.Line()
	}
	return lines
}

func ItemKey(field string, i int) string {
	return field + "." + strconv.Itoa(i)
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse(dateLayout, strings.TrimSpace(s))
	return t, err == nil
}

func (f Form) Validate(clientName func(id string) (string, bool)) (Input, map[string]string) {
	errs := map[string]string{}
	in := Input{Number: NormalizeNumber(f.Number), BillTo: strings.TrimSpace(f.BillTo), Note: strings.TrimRight(f.Note, " \t\r\n")}
	if in.Number == "" {
		errs["number"] = "Invoice number is required."
	} else if !ValidNumber(in.Number) {
		errs["number"] = "Use letters, digits, dots, dashes or underscores, up to 40 characters."
	}
	if id := strings.TrimSpace(f.ClientID); id != "" {
		name, ok := clientName(id)
		if !ok {
			errs["client_id"] = "Choose an existing client."
		} else {
			in.ClientID = &id
			if in.BillTo == "" {
				in.BillTo = name
			}
		}
	}
	if in.BillTo == "" {
		errs["bill_to"] = "Bill To is required."
	}
	if id := strings.TrimSpace(f.AssetID); id != "" {
		in.AssetID = &id
	}
	cur, err := money.ParseCurrency(f.Currency)
	if err != nil {
		errs["currency"] = "Choose IDR or USD."
	}
	in.Currency = cur
	if d, ok := parseDate(f.InvoiceDate); ok {
		in.InvoiceDate = d
	} else {
		errs["invoice_date"] = "Enter the invoice date."
	}
	if strings.TrimSpace(f.DueDate) != "" {
		d, ok := parseDate(f.DueDate)
		switch {
		case !ok:
			errs["due_date"] = "Enter a valid date."
		case !in.InvoiceDate.IsZero() && d.Before(in.InvoiceDate):
			errs["due_date"] = "The due date cannot be before the invoice date."
		default:
			in.DueDate = &d
		}
	}
	switch f.Status {
	case StatusPaid:
		in.Paid = true
		if d, ok := parseDate(f.PaidDate); ok {
			in.PaidDate = &d
		} else {
			errs["paid_date"] = "Enter the payment date."
		}
	case StatusUnpaid, "":
	default:
		errs["status"] = "Choose Unpaid or Paid."
	}
	in.Items, errs = validateItems(f.Items, errs)
	return in, errs
}

func validateItems(forms []ItemForm, errs map[string]string) ([]Item, map[string]string) {
	var items []Item
	for i, it := range forms {
		if it == (ItemForm{ProjectID: it.ProjectID}) {
			continue
		}
		item := Item{Title: strings.TrimSpace(it.Title), Description: strings.TrimRight(it.Description, " \t\r\n")}
		if id := strings.TrimSpace(it.ProjectID); id != "" {
			item.ProjectID = &id
		}
		if item.Title == "" {
			errs[ItemKey("item_title", i)] = "Item is required."
		}
		qty := strings.TrimSpace(it.Quantity)
		if qty == "" {
			qty = "1"
		}
		if q, err := money.Parse(qty); err != nil || q <= 0 || q.Cents() > 100000000 {
			errs[ItemKey("item_quantity", i)] = "Enter a quantity between 0 and 1,000,000."
		} else {
			item.Quantity = q
		}
		if r, err := money.Parse(it.Rate); err != nil || r < 0 {
			errs[ItemKey("item_rate", i)] = "Enter a rate of 0 or more."
		} else {
			item.Rate = r
		}
		tax := strings.TrimSuffix(strings.TrimSpace(it.Tax), "%")
		if tax == "" {
			tax = "0"
		}
		if t, err := money.Parse(tax); err != nil || t < 0 || t > money.FromCents(10000) {
			errs[ItemKey("item_tax", i)] = "Enter a tax between 0 and 100."
		} else {
			item.TaxPercent = t
		}
		if !item.Line().Fits() {
			errs[ItemKey("item_rate", i)] = "This line amount is too large."
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		errs["items"] = "Add at least one item."
	}
	if len(items) > MaxItems {
		errs["items"] = "An invoice can have at most 100 items."
	}
	return items, errs
}
