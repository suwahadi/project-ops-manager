package markdown

import (
	"strings"
	"testing"
)

func TestRenderSanitizes(t *testing.T) {
	r := New()
	out := string(r.Render("## Title\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1)) | a |\n\n- [x] done\n\n| h |\n|---|\n| c |\n\n<img src=x onerror=alert(1)>"))
	for _, bad := range []string{"<script", "javascript:", "onerror"} {
		if strings.Contains(out, bad) {
			t.Fatalf("output contains %q: %s", bad, out)
		}
	}
	for _, good := range []string{"<h2", "<table", "checkbox"} {
		if !strings.Contains(out, good) {
			t.Fatalf("output missing %q: %s", good, out)
		}
	}
}
