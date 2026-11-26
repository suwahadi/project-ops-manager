package project

import (
	"net/url"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

func TestApply(t *testing.T) {
	acme := "0f8fad5b-d9cb-469f-a165-70867728950e"
	acmeName := "Acme"
	site := "https://shop.example.com"
	done, failed := "done", "failed"
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []sqlc.ListProjectsRow{
		{Name: "Blog", ClientID: &acme, ClientName: &acmeName, ScreenshotStatus: &done, UpdatedAt: t0, AssetCount: 1, Currency: "IDR", ProjectValue: money.Some(money.FromCents(100000)), Received: money.FromCents(40000)},
		{Name: "admin", Url: &site, ScreenshotStatus: &failed, UpdatedAt: t0.Add(time.Hour), AssetCount: 4},
		{Name: "Docs", UpdatedAt: t0.Add(2 * time.Hour)},
	}
	isID := func(s string) bool { return s == acme }
	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"admin", "Blog", "Docs"}},
		{"q=acme", []string{"Blog"}},
		{"q=SHOP", []string{"admin"}},
		{"owner=internal", []string{"admin", "Docs"}},
		{"owner=" + acme, []string{"Blog"}},
		{"owner=nope", []string{"admin", "Blog", "Docs"}},
		{"screenshot=none", []string{"Docs"}},
		{"screenshot=failed", []string{"admin"}},
		{"sort=updated_desc", []string{"Docs", "admin", "Blog"}},
		{"sort=assets_desc", []string{"admin", "Blog", "Docs"}},
		{"sort=name_desc", []string{"Docs", "Blog", "admin"}},
		{"payment=none", []string{"admin", "Docs"}},
		{"payment=partial", []string{"Blog"}},
		{"payment=paid", nil},
		{"sort=value_desc", []string{"Blog", "admin", "Docs"}},
	}
	for _, c := range cases {
		v, _ := url.ParseQuery(c.query)
		var got []string
		for _, r := range Apply(rows, ParseFilter(v, isID)) {
			got = append(got, r.Name)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.query, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.query, got, c.want)
			}
		}
	}
}
