package assettype

import (
	"reflect"
	"testing"
)

func TestParseOffsets(t *testing.T) {
	got, err := ParseOffsets("7, 30 14,1")
	if err != nil || !reflect.DeepEqual(got, []int32{30, 14, 7, 1}) {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range []string{"", "0", "-1", "7,7", "a", "1.5"} {
		if _, err := ParseOffsets(bad); err == nil {
			t.Errorf("ParseOffsets(%q) expected error", bad)
		}
	}
}

func TestParseValues(t *testing.T) {
	fields := []Field{
		{Key: "plan", Label: "Plan", Type: FieldText, Required: true},
		{Key: "seats", Label: "Seats", Type: FieldNumber},
		{Key: "since", Label: "Since", Type: FieldDate},
		{Key: "panel", Label: "Panel", Type: FieldURL},
		{Key: "managed", Label: "Managed", Type: FieldBoolean},
		{Key: "tier", Label: "Tier", Type: FieldSelect, Options: []string{"Basic", "Pro"}},
	}
	form := map[string]string{"plan": "Business", "seats": "5", "since": "2026-01-02", "panel": "https://cp.example.com", "managed": "on", "tier": "Pro"}
	get := func(k string) (string, bool) { v, ok := form[k]; return v, ok }
	vals, errs := ParseValues(fields, get)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors %v", errs)
	}
	if vals["seats"] != 5.0 || vals["managed"] != true || vals["tier"] != "Pro" {
		t.Fatalf("values %v", vals)
	}

	bad := map[string]string{"seats": "five", "since": "02/01/2026", "panel": "ftp://x", "tier": "Gold"}
	get = func(k string) (string, bool) { v, ok := bad[k]; return v, ok }
	vals, errs = ParseValues(fields, get)
	for _, k := range []string{"plan", "seats", "since", "panel", "tier"} {
		if errs[k] == "" {
			t.Errorf("expected error for %s", k)
		}
	}
	if vals["managed"] != false {
		t.Errorf("unchecked boolean should be false, got %v", vals["managed"])
	}
}

func TestValuesRoundTrip(t *testing.T) {
	v := Values{"a": "x", "n": 3.5, "b": true}
	back := DecodeValues(v.Encode())
	if back.Input("a") != "x" || back.Input("n") != "3.5" || back.Input("b") != "true" {
		t.Fatalf("round trip %v", back)
	}
	if Slugify("SSL Certificate!") != "ssl-certificate" || KeyFromLabel("Control panel URL") != "control_panel_url" {
		t.Fatal("slug/key")
	}
}
