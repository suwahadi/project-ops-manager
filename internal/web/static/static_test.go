package static

import (
	"strings"
	"testing"
)

func TestURL(t *testing.T) {
	u := URL("js/app.js")
	if !strings.HasPrefix(u, "/static/js/app.js?v=") || len(u) != len("/static/js/app.js?v=")+12 {
		t.Fatalf("URL = %q", u)
	}
	if URL("js/app.js") != u {
		t.Fatal("URL is not stable")
	}
	if got := URL("missing.css"); got != "/static/missing.css" {
		t.Fatalf("missing file URL = %q", got)
	}
}
