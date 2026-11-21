package asset

import (
	"strconv"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

const OwnerInternal = "internal"

var RenewalPeriods = []int{1, 3, 6, 12, 24, 36}

type Form struct {
	TypeID         string
	ProviderID     string
	ProviderNew    string
	Owner          string
	Name           string
	URL            string
	OrderDate      string
	ExpirationDate string
	RenewalPeriod  string
	RenewalCustom  string
	AutoRenew      bool
	CostPrice      string
	SellingPrice   string
	Currency       string
	RDAPEnabled    bool
	Custom         map[string]string
	CustomPresent  map[string]bool
}

type Input struct {
	TypeID        string
	ProviderID    string
	ProviderName  string
	ClientID      *string
	Name          string
	URL           *string
	OrderDate     *time.Time
	Expiration    time.Time
	RenewalMonths int
	AutoRenew     bool
	Cost          money.Amount
	Selling       money.NullAmount
	Currency      money.Currency
	Custom        assettype.Values
	RDAPEnabled   bool
}

func (f Form) IsCustomPeriod() bool {
	if f.RenewalPeriod == "custom" {
		return true
	}
	if f.RenewalPeriod == "" {
		return false
	}
	n, err := strconv.Atoi(f.RenewalPeriod)
	if err != nil {
		return true
	}
	for _, p := range RenewalPeriods {
		if p == n {
			return false
		}
	}
	return true
}

func parseDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	return t, err == nil
}

func (f Form) Validate(fields []assettype.Field, supportsRDAP bool) (Input, map[string]string) {
	errs := map[string]string{}
	in := Input{TypeID: f.TypeID, AutoRenew: f.AutoRenew, RDAPEnabled: supportsRDAP && f.RDAPEnabled}

	in.Name = strings.TrimSpace(f.Name)
	if in.Name == "" {
		errs["name"] = "Name is required."
	} else if len(in.Name) > 255 {
		errs["name"] = "Name must be at most 255 characters."
	}
	if f.TypeID == "" {
		errs["type_id"] = "Choose an asset type."
	}
	if strings.TrimSpace(f.ProviderNew) != "" {
		in.ProviderName = strings.TrimSpace(f.ProviderNew)
	} else if f.ProviderID != "" {
		in.ProviderID = f.ProviderID
	} else {
		errs["provider_id"] = "Choose a provider or enter a new one."
	}
	if f.Owner != "" && f.Owner != OwnerInternal {
		id := f.Owner
		in.ClientID = &id
	}
	if u := strings.TrimSpace(f.URL); u != "" {
		if !assettype.ValidURL(u) {
			errs["url"] = "Enter a valid http or https URL."
		}
		in.URL = &u
	}
	if strings.TrimSpace(f.OrderDate) != "" {
		t, ok := parseDate(f.OrderDate)
		if !ok {
			errs["order_date"] = "Enter a valid date."
		}
		in.OrderDate = &t
	}
	if t, ok := parseDate(f.ExpirationDate); ok {
		in.Expiration = t
	} else {
		errs["expiration_date"] = "Expiration date is required."
	}
	period := f.RenewalPeriod
	if period == "custom" {
		period = f.RenewalCustom
	}
	if n, err := strconv.Atoi(strings.TrimSpace(period)); err == nil && n > 0 && n <= 120 {
		in.RenewalMonths = n
	} else {
		errs["renewal_period"] = "Renewal period must be a whole number of months between 1 and 120."
	}
	cur, err := money.ParseCurrency(f.Currency)
	if err != nil {
		errs["currency"] = "Choose IDR or USD."
	}
	in.Currency = cur
	cost, err := money.Parse(f.CostPrice)
	if err != nil || cost < 0 {
		errs["cost_price"] = "Enter a cost price of 0 or more, e.g. 180000 or 12.50."
	}
	in.Cost = cost
	if strings.TrimSpace(f.SellingPrice) != "" {
		sell, err := money.Parse(f.SellingPrice)
		switch {
		case err != nil || sell < 0:
			errs["selling_price"] = "Enter a selling price of 0 or more."
		case in.ClientID == nil:
			errs["selling_price"] = "Internal assets have no selling price. Clear it or choose a client."
		}
		in.Selling = money.Some(sell)
	}
	vals, fieldErrs := assettype.ParseValues(fields, func(k string) (string, bool) {
		return f.Custom[k], f.CustomPresent[k]
	})
	for k, v := range fieldErrs {
		errs["cf_"+k] = v
	}
	in.Custom = vals
	return in, errs
}

func HasCustomValues(v assettype.Values) bool {
	for _, val := range v {
		switch x := val.(type) {
		case nil:
		case string:
			if x != "" {
				return true
			}
		case bool:
			if x {
				return true
			}
		default:
			return true
		}
	}
	return false
}
