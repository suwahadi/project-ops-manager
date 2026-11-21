package client

import (
	"cmp"
	"net/url"
	"slices"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

type Option struct{ Value, Label string }

var UsageOptions = []Option{
	{"", "All clients"},
	{"assets", "With assets"},
	{"projects", "With projects"},
	{"unused", "No assets or projects"},
}

var SortOptions = []Option{
	{"", "Name (A–Z)"},
	{"name_desc", "Name (Z–A)"},
	{"assets_desc", "Most assets"},
	{"projects_desc", "Most projects"},
	{"created_desc", "Recently added"},
}

type Filter struct {
	Query string
	Usage string
	Sort  string
}

func validOption(opts []Option, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return v
		}
	}
	return ""
}

func ParseFilter(q url.Values) Filter {
	return Filter{
		Query: strings.TrimSpace(q.Get("q")),
		Usage: validOption(UsageOptions, q.Get("usage")),
		Sort:  validOption(SortOptions, q.Get("sort")),
	}
}

func (f Filter) Filtered() bool { return f.Query != "" || f.Usage != "" }

func (f Filter) Active() int {
	n := 0
	for _, v := range []string{f.Usage, f.Sort} {
		if v != "" {
			n++
		}
	}
	return n
}

func optionalText(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func matches(row sqlc.ListClientsRow, f Filter) bool {
	switch f.Usage {
	case "assets":
		if row.AssetCount == 0 {
			return false
		}
	case "projects":
		if row.ProjectCount == 0 {
			return false
		}
	case "unused":
		if row.AssetCount > 0 || row.ProjectCount > 0 {
			return false
		}
	}
	if f.Query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{row.Name, optionalText(row.ContactPerson), optionalText(row.Email), optionalText(row.Phone)}, " "))
	return strings.Contains(hay, strings.ToLower(f.Query))
}

func Apply(rows []sqlc.ListClientsRow, f Filter) []sqlc.ListClientsRow {
	out := make([]sqlc.ListClientsRow, 0, len(rows))
	for _, r := range rows {
		if matches(r, f) {
			out = append(out, r)
		}
	}
	byName := func(a, b sqlc.ListClientsRow) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	slices.SortStableFunc(out, func(a, b sqlc.ListClientsRow) int {
		switch f.Sort {
		case "name_desc":
			return byName(b, a)
		case "assets_desc":
			return cmp.Or(cmp.Compare(b.AssetCount, a.AssetCount), byName(a, b))
		case "projects_desc":
			return cmp.Or(cmp.Compare(b.ProjectCount, a.ProjectCount), byName(a, b))
		case "created_desc":
			return b.CreatedAt.Compare(a.CreatedAt)
		}
		return byName(a, b)
	})
	return out
}
