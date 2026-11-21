package asset_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/suwahadi/project-ops-manager/internal/asset"
	"github.com/suwahadi/project-ops-manager/internal/assettype"
	"github.com/suwahadi/project-ops-manager/internal/credential"
	"github.com/suwahadi/project-ops-manager/internal/crypto"
	"github.com/suwahadi/project-ops-manager/internal/db/dbtest"
	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

func typeBySlug(t *testing.T, q *sqlc.Queries, slug string) sqlc.AssetType {
	t.Helper()
	at, err := q.GetAssetTypeBySlug(context.Background(), slug)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestServiceIntegration(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	q := sqlc.New(pool)
	svc := asset.NewService(pool, t.TempDir())
	today := mustDate("2026-11-21")
	domain := typeBySlug(t, q, "domain")
	vps := typeBySlug(t, q, "vps")

	mk := func(name, exp string, typeID string, custom assettype.Values) string {
		id, err := svc.Create(ctx, asset.Input{
			TypeID: typeID, ProviderName: "Registrar", Name: name, Expiration: mustDate(exp), RenewalMonths: 12,
			Cost: money.FromCents(18000000), Selling: money.Some(money.FromCents(25000050)), Currency: money.IDR, Custom: custom,
		})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return id
	}
	expired := mk("expired.com", "2026-11-20", domain.ID, nil)
	expiring := mk("soon.com", "2026-12-21", domain.ID, nil)
	active := mk("later.com", "2026-12-22", domain.ID, nil)
	vpsSoon := mk("vps-soon", "2026-12-05", vps.ID, assettype.Values{"ip_address": "203.0.113.1"})
	cancelled := mk("gone.com", "2027-05-01", domain.ID, nil)
	if err := svc.Cancel(ctx, cancelled, today); err != nil {
		t.Fatal(err)
	}

	want := map[asset.Status][]string{
		asset.StatusExpired:      {expired},
		asset.StatusExpiringSoon: {vpsSoon, expiring},
		asset.StatusActive:       {active},
		asset.StatusCancelled:    {cancelled},
	}
	for status, ids := range want {
		items, err := svc.List(ctx, asset.Filter{Status: string(status)}, today)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != len(ids) {
			t.Fatalf("status %s: got %d items, want %d", status, len(items), len(ids))
		}
		for i, it := range items {
			if it.ID != ids[i] || it.Status != status {
				t.Fatalf("status %s item %d: got %s/%s", status, i, it.Name, it.Status)
			}
		}
	}

	d, err := svc.Get(ctx, expiring, today)
	if err != nil {
		t.Fatal(err)
	}
	if d.CostPrice != money.FromCents(18000000) || !d.SellingPrice.Valid || d.SellingPrice.Amount != money.FromCents(25000050) {
		t.Fatalf("money round trip: %v %v", d.CostPrice, d.SellingPrice)
	}
	if m, ok := d.Margin(); !ok || m != money.FromCents(7000050) {
		t.Fatalf("margin = %v", m)
	}

	items, err := svc.List(ctx, asset.Filter{Query: "100%_"}, today)
	if err != nil || len(items) != 0 {
		t.Fatalf("like escaping: %v %v", items, err)
	}

	in := asset.Input{TypeID: domain.ID, ProviderName: "Registrar", Name: "vps-soon", Expiration: mustDate("2026-12-05"), RenewalMonths: 1, Cost: 1, Currency: money.USD, Custom: assettype.Values{}}
	if err := svc.Update(ctx, vpsSoon, in); !errors.Is(err, asset.ErrTypeLocked) {
		t.Fatalf("expected type lock, got %v", err)
	}

	key := bytes.Repeat([]byte{7}, 32)
	box, _ := crypto.NewBox(key, 1)
	creds := credential.NewService(pool, box)
	if err := creds.Create(ctx, active, credential.Input{Label: "Panel", Username: "root", Password: "hunter2-password"}); err != nil {
		t.Fatal(err)
	}
	list, err := creds.List(ctx, active)
	if err != nil || len(list) != 1 || !list[0].HasPassword {
		t.Fatalf("list credentials: %v %v", list, err)
	}
	raw, err := q.GetCredential(ctx, sqlc.GetCredentialParams{ID: list[0].ID, AssetID: active})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw.PasswordCiphertext, []byte("hunter2")) || len(raw.PasswordNonce) != crypto.NonceSize {
		t.Fatal("password is not encrypted at rest")
	}
	plain, err := creds.Reveal(ctx, active, list[0].ID)
	if err != nil || plain != "hunter2-password" {
		t.Fatalf("reveal: %q %v", plain, err)
	}
	if _, err := creds.Reveal(ctx, expired, list[0].ID); !errors.Is(err, credential.ErrNotFound) {
		t.Fatalf("reveal through another asset should fail, got %v", err)
	}

	if _, err := svc.Delete(ctx, active); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, active, today); !errors.Is(err, asset.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}
