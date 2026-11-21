package asset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/document"
	"github.com/suwahadi/project-ops-manager/internal/money"
)

type RenewInput struct {
	NewExpiration time.Time
	Cost          money.Amount
	Selling       money.NullAmount
	PaidOn        time.Time
}

func DefaultNewExpiration(current time.Time, periodMonths int) time.Time {
	return AddMonths(current, periodMonths)
}

func (s *Service) Renew(ctx context.Context, assetID string, in RenewInput, docs []document.Prepared) (string, error) {
	var renewalID string
	err := s.inTx(ctx, func(q *sqlc.Queries) error {
		cur, err := q.GetAssetForUpdate(ctx, assetID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock asset: %w", err)
		}
		renewalID, err = q.CreateRenewal(ctx, sqlc.CreateRenewalParams{
			AssetID: assetID, PaidOn: in.PaidOn, PreviousExpirationDate: cur.ExpirationDate,
			NewExpirationDate: in.NewExpiration, CostPrice: in.Cost, SellingPrice: in.Selling, Currency: cur.Currency,
		})
		if err != nil {
			return fmt.Errorf("create renewal: %w", err)
		}
		if err := q.ApplyRenewalToAsset(ctx, sqlc.ApplyRenewalToAssetParams{ID: assetID, ExpirationDate: in.NewExpiration, CostPrice: in.Cost, SellingPrice: in.Selling}); err != nil {
			return fmt.Errorf("update asset: %w", err)
		}
		for _, d := range docs {
			id := renewalID
			if err := document.Insert(ctx, q, nil, &id, d); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return renewalID, nil
}

func (s *Service) Renewals(ctx context.Context, assetID string) ([]sqlc.Renewal, error) {
	return s.q.ListRenewalsByAsset(ctx, assetID)
}

func (s *Service) AttachDocuments(ctx context.Context, assetID string, docs []document.Prepared) error {
	return s.inTx(ctx, func(q *sqlc.Queries) error {
		for _, d := range docs {
			id := assetID
			if err := document.Insert(ctx, q, &id, nil, d); err != nil {
				return err
			}
		}
		return nil
	})
}
