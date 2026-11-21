package client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/client"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
	"github.com/suwahadi/project-ops-manager/internal/project"
)

func TestLinkAndUnlink(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	clients := client.NewService(pool)
	assets := asset.NewService(pool, t.TempDir())
	projects := project.NewService(pool, time.Now)

	acme, err := clients.Create(ctx, client.Input{Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := clients.Create(ctx, client.Input{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}
	domain, err := q.GetAssetTypeBySlug(ctx, "domain")
	if err != nil {
		t.Fatal(err)
	}
	mkAsset := func(name string, owner *string, selling money.NullAmount) string {
		id, err := assets.Create(ctx, asset.Input{
			TypeID: domain.ID, ProviderName: "Registrar", ClientID: owner, Name: name,
			Expiration: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), RenewalMonths: 12,
			Cost: money.FromCents(10000), Selling: selling, Currency: money.IDR,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return id
	}
	internal := mkAsset("internal.com", nil, money.NullAmount{})
	moved := mkAsset("moved.com", &other, money.Some(money.FromCents(20000)))
	own := mkAsset("own.com", &acme, money.Some(money.FromCents(30000)))

	linkable, err := clients.LinkableAssets(ctx, acme)
	if err != nil {
		t.Fatal(err)
	}
	if len(linkable) != 2 || linkable[0].ID != internal || linkable[1].ID != moved || deref(linkable[1].ClientName) != "Other" {
		t.Fatalf("linkable assets: %+v", linkable)
	}

	n, err := clients.LinkAssets(ctx, acme, []string{internal, moved, own})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("linked %d assets, want 2", n)
	}
	rows, err := clients.Assets(ctx, acme)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("client has %d assets, want 3", len(rows))
	}

	if err := clients.UnlinkAsset(ctx, acme, moved); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetAsset(ctx, moved)
	if err != nil {
		t.Fatal(err)
	}
	if a.ClientID != nil || a.SellingPrice.Valid {
		t.Fatalf("unlinked asset kept owner or selling price: %+v", a)
	}
	if err := clients.UnlinkAsset(ctx, other, own); !errors.Is(err, client.ErrNotLinked) {
		t.Fatalf("unlink from wrong client: got %v", err)
	}

	p, err := projects.Create(ctx, project.Input{Name: "Site", Brief: project.BriefTemplate})
	if err != nil {
		t.Fatal(err)
	}
	lp, err := clients.LinkableProjects(ctx, acme)
	if err != nil {
		t.Fatal(err)
	}
	if len(lp) != 1 || lp[0].ID != p || lp[0].ClientName != nil {
		t.Fatalf("linkable projects: %+v", lp)
	}
	if n, err := clients.LinkProjects(ctx, acme, []string{p}); err != nil || n != 1 {
		t.Fatalf("link project: %d %v", n, err)
	}
	if err := clients.Delete(ctx, acme); !errors.Is(err, client.ErrInUse) {
		t.Fatalf("delete client with links: got %v", err)
	}
	if err := clients.UnlinkProject(ctx, acme, p); err != nil {
		t.Fatal(err)
	}
	if err := clients.UnlinkProject(ctx, acme, p); !errors.Is(err, client.ErrNotLinked) {
		t.Fatalf("second unlink: got %v", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
