package rdap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const sampleResponse = `{
  "objectClassName": "domain",
  "ldhName": "EXAMPLE.COM",
  "events": [
    {"eventAction": "registration", "eventDate": "1995-08-14T04:00:00Z"},
    {"eventAction": "expiration", "eventDate": "2027-08-13T04:00:00Z"},
    {"eventAction": "last update of RDAP database", "eventDate": "2026-11-21T10:00:00Z"}
  ]
}`

func TestParseExpiration(t *testing.T) {
	got, err := ParseExpiration([]byte(sampleResponse))
	if err != nil || !got.Equal(time.Date(2027, 8, 13, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := ParseExpiration([]byte(`{"events":[{"eventAction":"registration","eventDate":"2020-01-01T00:00:00Z"}]}`)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("missing expiration should be unsupported, got %v", err)
	}
	if got, err := ParseExpiration([]byte(`{"events":[{"eventAction":"expiration","eventDate":"2027-01-31T23:59:59.000+00:00"}]}`)); err != nil || got.Day() != 31 {
		t.Fatalf("fractional seconds: %v %v", got, err)
	}
	if _, err := ParseExpiration([]byte(`not json`)); err == nil || errors.Is(err, ErrUnsupported) {
		t.Fatalf("garbage should be an error, got %v", err)
	}
}

func TestNormalizeDomain(t *testing.T) {
	cases := map[string]string{"Example.COM": "example.com", "https://www.example.co.id/path": "example.co.id", "example.com.": "example.com"}
	for in, want := range cases {
		if got, err := NormalizeDomain(in); err != nil || got != want {
			t.Errorf("NormalizeDomain(%q) = %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"Production VPS", "localhost", "bücher.de"} {
		if _, err := NormalizeDomain(bad); err == nil {
			t.Errorf("NormalizeDomain(%q) expected error", bad)
		}
	}
}

func TestClientLookupRateLimitAndBootstrapCache(t *testing.T) {
	var mu sync.Mutex
	bootstrapHits := 0
	var rdapSrv *httptest.Server
	rdapSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dns.json":
			mu.Lock()
			bootstrapHits++
			mu.Unlock()
			_, _ = w.Write([]byte(`{"services":[[["com","net"],["` + rdapSrv.URL + `/rdap"]],[["co.id"],["` + rdapSrv.URL + `/id/"]]]}`))
		case "/rdap/domain/example.com":
			_, _ = w.Write([]byte(sampleResponse))
		case "/id/domain/toko.co.id":
			_, _ = w.Write([]byte(`{"events":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer rdapSrv.Close()

	now := time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)
	var slept []time.Duration
	c := NewClient()
	c.BootstrapURL = rdapSrv.URL + "/dns.json"
	c.HTTP = rdapSrv.Client()
	c.Now = func() time.Time { return now }
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }

	ctx := context.Background()
	exp, err := c.Expiration(ctx, "example.com")
	if err != nil || exp.Year() != 2027 {
		t.Fatalf("lookup: %v %v", exp, err)
	}
	if _, err := c.Expiration(ctx, "missing.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := c.Expiration(ctx, "toko.co.id"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("no expiration event should be unsupported, got %v", err)
	}
	if _, err := c.Expiration(ctx, "example.zz"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unknown TLD should be unsupported, got %v", err)
	}
	if bootstrapHits != 1 {
		t.Fatalf("bootstrap should be cached, fetched %d times", bootstrapHits)
	}
	if len(slept) != 3 || slept[0] != 0 || slept[1] != time.Second || slept[2] != 2*time.Second {
		t.Fatalf("rate limit waits = %v", slept)
	}
	now = now.Add(25 * time.Hour)
	if _, err := c.Expiration(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if bootstrapHits != 2 {
		t.Fatalf("bootstrap should refresh after 24h, fetched %d times", bootstrapHits)
	}
}
