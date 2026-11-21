package screenshot

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	ips, ok := f[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	var out []netip.Addr
	for _, s := range ips {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func TestGuard(t *testing.T) {
	g := &Guard{Resolver: fakeResolver{
		"example.com":   {"93.184.215.14"},
		"internal.corp": {"10.0.0.5"},
		"mixed.example": {"93.184.215.14", "127.0.0.1"},
		"v6.example":    {"2606:4700::1111"},
	}}
	ctx := context.Background()
	allowed := []string{"https://example.com/", "http://example.com:8080/x", "https://93.184.215.14", "https://v6.example"}
	for _, u := range allowed {
		if err := g.CheckURL(ctx, u); err != nil {
			t.Errorf("%s should be allowed: %v", u, err)
		}
	}
	blocked := []string{
		"ftp://example.com", "file:///etc/passwd", "javascript:alert(1)",
		"http://localhost", "http://127.0.0.1", "http://[::1]/", "http://10.1.2.3", "http://192.168.1.1",
		"http://172.16.0.1", "http://169.254.169.254/latest/meta-data", "http://[fe80::1]", "http://0.0.0.0",
		"http://100.64.1.1", "http://[::ffff:127.0.0.1]", "http://internal.corp", "http://mixed.example",
		"http://chromium.internal", "http://unknown.example",
	}
	for _, u := range blocked {
		if err := g.CheckURL(ctx, u); err == nil {
			t.Errorf("%s should be blocked", u)
		}
	}
	if !g.AllowRequest(ctx, "data:image/png;base64,AAAA") || g.AllowRequest(ctx, "http://10.0.0.1/") || g.AllowRequest(ctx, "chrome://settings") {
		t.Fatal("AllowRequest mismatch")
	}
}
