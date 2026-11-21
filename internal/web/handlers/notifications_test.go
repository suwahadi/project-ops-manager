package handlers

import "testing"

func TestValidChatID(t *testing.T) {
	for _, ok := range []string{"123456789", "-1001234567890", "@my_channel"} {
		if !validChatID(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"@", "abc", "12 34", "@my channel"} {
		if validChatID(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestPlainAddress(t *testing.T) {
	if !plainAddress("ops@example.com") {
		t.Fatal("plain address rejected")
	}
	for _, bad := range []string{"Ops <ops@example.com>", "not-an-email", " ops@example.com"} {
		if plainAddress(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
