package templates

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/money"
)

func baseFuncs() template.FuncMap {
	return template.FuncMap{
		"date":     FormatDate,
		"datePtr":  formatDatePtr,
		"dateTime": FormatDateTime,
		"isoDate":  isoDate,
		"money":    formatMoney,
		"nullMoney": func(n money.NullAmount, cur string) string {
			if !n.Valid {
				return "—"
			}
			return n.Amount.Format(money.Currency(cur))
		},
		"amountInput": func(a money.Amount) string { return a.String() },
		"icon":        Icon,
		"navItems":    func() []NavItem { return navItems },
		"iconNames":   func() []string { return IconNames },
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict needs pairs")
			}
			m := map[string]any{}
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"list":     func(v ...any) []any { return v },
		"contains": func(list []string, s string) bool { return containsString(list, s) },
		"join":     strings.Join,
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
		"add": func(a, b int) int { return a + b },
		"int": func(v any) int {
			switch n := v.(type) {
			case int:
				return n
			case int32:
				return int(n)
			case int64:
				return int(n)
			}
			return 0
		},
		"yesNo": func(b bool) string {
			if b {
				return "Yes"
			}
			return "No"
		},
		"margin": func(cost money.Amount, selling money.NullAmount, cur string) string {
			if !selling.Valid {
				return "—"
			}
			return (selling.Amount - cost).Format(money.Currency(cur))
		},
		"bytes": func(n int64) string {
			switch {
			case n >= 1<<20:
				return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
			case n >= 1<<10:
				return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
			}
			return fmt.Sprintf("%d B", n)
		},
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func FormatDate(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("02 Jan 2006")
}

func formatDatePtr(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return FormatDate(*t)
}

func FormatDateTime(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return "—"
	}
	if loc != nil {
		t = t.In(loc)
	}
	return t.Format("02 Jan 2006 15:04")
}

func isoDate(v any) string {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	case *time.Time:
		if t == nil || t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	}
	return ""
}

func formatMoney(a money.Amount, cur any) string {
	return a.Format(money.Currency(fmt.Sprint(cur)))
}

type NavItem struct {
	Key   string
	Label string
	Href  string
	Icon  string
}

var navItems = []NavItem{
	{"dashboard", "Dashboard", "/", "dashboard"},
	{"assets", "Assets", "/assets", "assets"},
	{"clients", "Clients", "/clients", "clients"},
	{"projects", "Projects", "/projects", "projects"},
	{"finance", "Finance", "/finance", "finance"},
	{"settings", "Settings", "/settings", "settings"},
}
