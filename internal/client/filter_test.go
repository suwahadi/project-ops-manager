package client

import (
	"net/url"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

func names(rows []sqlc.ListClientsRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

func TestApply(t *testing.T) {
	email := "ops@beta.test"
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []sqlc.ListClientsRow{
		{Name: "alpha", AssetCount: 1, CreatedAt: t0},
		{Name: "Beta", Email: &email, ProjectCount: 2, CreatedAt: t0.Add(time.Hour)},
		{Name: "Gamma", AssetCount: 3, ProjectCount: 1, CreatedAt: t0.Add(2 * time.Hour)},
		{Name: "delta", CreatedAt: t0.Add(3 * time.Hour)},
	}
	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"alpha", "Beta", "delta", "Gamma"}},
		{"q=OPS@", []string{"Beta"}},
		{"usage=assets", []string{"alpha", "Gamma"}},
		{"usage=projects&sort=projects_desc", []string{"Beta", "Gamma"}},
		{"usage=unused", []string{"delta"}},
		{"sort=assets_desc", []string{"Gamma", "alpha", "Beta", "delta"}},
		{"sort=created_desc", []string{"delta", "Gamma", "Beta", "alpha"}},
		{"sort=name_desc", []string{"Gamma", "delta", "Beta", "alpha"}},
		{"usage=bogus&sort=bogus", []string{"alpha", "Beta", "delta", "Gamma"}},
	}
	for _, c := range cases {
		v, _ := url.ParseQuery(c.query)
		got := names(Apply(rows, ParseFilter(v)))
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.query, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.query, got, c.want)
			}
		}
	}
	if f := ParseFilter(url.Values{"usage": {"assets"}, "sort": {"name_desc"}}); f.Active() != 2 || !f.Filtered() {
		t.Fatalf("active/filtered: %+v", f)
	}
}
