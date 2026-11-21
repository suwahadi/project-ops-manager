package project

import (
	"cmp"
	"net/url"
	"slices"
	"strings"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

const OwnerInternal = "internal"

type Option struct{ Value, Label string }

var ScreenshotOptions = []Option{
	{"", "All screenshots"},
	{"done", "Captured"},
	{"pending", "Capturing"},
	{"failed", "Failed"},
	{"none", "No screenshot"},
}

var SortOptions = []Option{
	{"", "Name (A–Z)"},
	{"name_desc", "Name (Z–A)"},
	{"updated_desc", "Recently updated"},
	{"assets_desc", "Most assets"},
}

type Filter struct {
	Query      string
	Owner      string
	Screenshot string
	Sort       string
}

func validOption(opts []Option, v string) string {
	for _, o := range opts {
		if o.Value == v {
			return v
		}
	}
	return ""
}

func ParseFilter(q url.Values, isID func(string) bool) Filter {
	f := Filter{
		Query:      strings.TrimSpace(q.Get("q")),
		Owner:      q.Get("owner"),
		Screenshot: validOption(ScreenshotOptions, q.Get("screenshot")),
		Sort:       validOption(SortOptions, q.Get("sort")),
	}
	if f.Owner != OwnerInternal && !isID(f.Owner) {
		f.Owner = ""
	}
	return f
}

func (f Filter) Filtered() bool { return f.Query != "" || f.Owner != "" || f.Screenshot != "" }

func (f Filter) Active() int {
	n := 0
	for _, v := range []string{f.Owner, f.Screenshot, f.Sort} {
		if v != "" {
			n++
		}
	}
	return n
}

func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func matches(row sqlc.ListProjectsRow, f Filter) bool {
	switch {
	case f.Owner == OwnerInternal && row.ClientID != nil:
		return false
	case f.Owner != "" && f.Owner != OwnerInternal && text(row.ClientID) != f.Owner:
		return false
	}
	if f.Screenshot != "" {
		status := text(row.ScreenshotStatus)
		if f.Screenshot == "none" {
			if status != "" {
				return false
			}
		} else if status != f.Screenshot {
			return false
		}
	}
	if f.Query == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{row.Name, text(row.Url), text(row.ClientName)}, " "))
	return strings.Contains(hay, strings.ToLower(f.Query))
}

func Apply(rows []sqlc.ListProjectsRow, f Filter) []sqlc.ListProjectsRow {
	out := make([]sqlc.ListProjectsRow, 0, len(rows))
	for _, r := range rows {
		if matches(r, f) {
			out = append(out, r)
		}
	}
	byName := func(a, b sqlc.ListProjectsRow) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	}
	slices.SortStableFunc(out, func(a, b sqlc.ListProjectsRow) int {
		switch f.Sort {
		case "name_desc":
			return byName(b, a)
		case "updated_desc":
			return b.UpdatedAt.Compare(a.UpdatedAt)
		case "assets_desc":
			return cmp.Or(cmp.Compare(b.AssetCount, a.AssetCount), byName(a, b))
		}
		return byName(a, b)
	})
	return out
}
