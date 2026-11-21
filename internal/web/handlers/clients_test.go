package handlers

import (
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestSelectedIDs(t *testing.T) {
	a := "0f8fad5b-d9cb-469f-a165-70867728950e"
	b := "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	form := url.Values{"asset_ids": {a, "not-a-uuid", b, a, ""}}
	r := httptest.NewRequest("POST", "/clients/x/assets", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	got := selectedIDs(r, "asset_ids")
	if want := []string{a, b}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selectedIDs = %v, want %v", got, want)
	}
}

func TestLinkedMessage(t *testing.T) {
	if got := linkedMessage(1, "asset", "assets"); got != "1 asset linked." {
		t.Fatal(got)
	}
	if got := linkedMessage(0, "asset", "assets"); got != "0 assets linked." {
		t.Fatal(got)
	}
}
