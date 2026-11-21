package rdap_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/rdap"
)

func day(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func TestRunOf100DomainsHonoursRateLimitAndTimeout(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/dns.json":
			fmt.Fprintf(w, `{"services":[[["com"],["%s/"]],[["net"],["%s/net/"]]]}`, srv.URL, srv.URL)
		case strings.HasPrefix(r.URL.Path, "/net/"):
			time.Sleep(200 * time.Millisecond)
		default:
			_, _ = w.Write([]byte(`{"events":[{"eventAction":"expiration","eventDate":"2027-08-13T04:00:00Z"}]}`))
		}
	}))
	defer srv.Close()

	var mu sync.Mutex
	clock := time.Date(2026, 11, 21, 3, 0, 0, 0, time.UTC)
	var waits []time.Duration
	c := rdap.NewClient()
	c.BootstrapURL = srv.URL + "/dns.json"
	c.HTTP = srv.Client()
	c.Timeout = 50 * time.Millisecond
	c.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	c.Sleep = func(_ context.Context, d time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		waits = append(waits, d)
		clock = clock.Add(d)
		return nil
	}
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		if _, err := c.Expiration(ctx, fmt.Sprintf("site%d.com", i)); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	for i, w := range waits {
		if i > 0 && w != time.Second {
			t.Fatalf("request %d waited %v, want 1s", i, w)
		}
	}
	start := time.Now()
	_, err := c.Expiration(ctx, "slow.net")
	if err == nil || time.Since(start) > 150*time.Millisecond {
		t.Fatalf("timeout not enforced: %v after %v", err, time.Since(start))
	}
}

type fakeLookup map[string]any

func (f fakeLookup) Expiration(_ context.Context, name string) (time.Time, error) {
	switch v := f[name].(type) {
	case time.Time:
		return v, nil
	case error:
		return time.Time{}, v
	}
	return time.Time{}, rdap.ErrUnsupported
}

func TestServiceRun(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	domain, _ := q.GetAssetTypeBySlug(ctx, "domain")
	vps, _ := q.GetAssetTypeBySlug(ctx, "vps")
	assets := asset.NewService(pool, t.TempDir())
	mk := func(name string, typeID string, exp string, rdapOn bool) string {
		id, err := assets.Create(ctx, asset.Input{TypeID: typeID, ProviderName: "Reg", Name: name, Expiration: day(exp), RenewalMonths: 12,
			Cost: money.FromCents(1), Currency: money.IDR, Custom: assettype.Values{}, RDAPEnabled: rdapOn})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	changed := mk("changed.com", domain.ID, "2026-12-01", true)
	same := mk("same.com", domain.ID, "2027-03-03", true)
	unsupported := mk("shop.zz", domain.ID, "2027-01-01", true)
	broken := mk("broken.com", domain.ID, "2027-02-02", true)
	off := mk("off.com", domain.ID, "2027-02-02", false)
	server := mk("server", vps.ID, "2027-02-02", false)

	lookup := fakeLookup{
		"changed.com": day("2027-12-01"),
		"same.com":    day("2027-03-03"),
		"broken.com":  fmt.Errorf("RDAP server returned HTTP 500"),
		"off.com":     day("2030-01-01"),
	}
	svc := rdap.NewService(pool, lookup, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	sum, err := svc.RunAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Checked != 4 || sum.Updated != 1 || sum.Unchanged != 1 || sum.Unsupported != 1 || sum.Errors != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	today := day("2026-11-21")
	get := func(id string) asset.Detail {
		d, err := assets.Get(ctx, id, today)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := get(changed); !d.ExpirationDate.Equal(day("2027-12-01")) || *d.RdapResult != "updated" {
		t.Fatalf("changed: %v %v", d.ExpirationDate, d.RdapResult)
	}
	if d := get(same); *d.RdapResult != "unchanged" {
		t.Fatal("same should be unchanged")
	}
	if d := get(unsupported); *d.RdapResult != "unsupported" || !d.ExpirationDate.Equal(day("2027-01-01")) {
		t.Fatal("unsupported must keep the date")
	}
	if d := get(broken); *d.RdapResult != "error" || d.RdapError == nil || !d.ExpirationDate.Equal(day("2027-02-02")) {
		t.Fatal("error must keep the date and store the message")
	}
	if d := get(off); d.RdapResult != nil || !d.ExpirationDate.Equal(day("2027-02-02")) {
		t.Fatal("RDAP-disabled domains are skipped")
	}
	if _, err := svc.CheckAsset(ctx, server); err != rdap.ErrNotEligible {
		t.Fatalf("non-domain check should be rejected, got %v", err)
	}
	run, err := q.LatestJobRun(ctx, rdap.JobName)
	if err != nil || run.Status != "failed" || run.Error == nil {
		t.Fatalf("job run should be failed due to one error: %+v %v", run, err)
	}
}
