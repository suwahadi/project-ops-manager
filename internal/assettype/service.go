package assettype

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
)

var (
	ErrNotFound      = errors.New("asset type not found")
	ErrBuiltin       = errors.New("built-in types cannot be deleted")
	ErrInUse         = errors.New("type has assets")
	ErrDuplicate     = errors.New("name or key already exists")
	ErrFieldNotFound = errors.New("field not found")
)

type Type struct {
	ID           string
	Name         string
	Slug         string
	Icon         string
	Builtin      bool
	SupportsRDAP bool
	Offsets      []int32
	AssetCount   int64
	Fields       []Field
}

func (t Type) OffsetsText() string { return FormatOffsets(t.Offsets) }

type Service struct {
	pool *pgxpool.Pool
	q    *sqlc.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: sqlc.New(pool)}
}

func fieldFromRow(r sqlc.AssetTypeField) Field {
	var opts []string
	_ = json.Unmarshal(r.Options, &opts)
	return Field{ID: r.ID, TypeID: r.AssetTypeID, Key: r.Key, Label: r.Label, Type: FieldType(r.FieldType), Options: opts, Required: r.IsRequired, SortOrder: int(r.SortOrder)}
}

func (s *Service) List(ctx context.Context) ([]Type, error) {
	rows, err := s.q.ListAssetTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list asset types: %w", err)
	}
	fieldRows, err := s.q.ListAllAssetTypeFields(ctx)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	byType := map[string][]Field{}
	for _, f := range fieldRows {
		byType[f.AssetTypeID] = append(byType[f.AssetTypeID], fieldFromRow(f))
	}
	out := make([]Type, len(rows))
	for i, r := range rows {
		out[i] = Type{ID: r.ID, Name: r.Name, Slug: r.Slug, Icon: r.Icon, Builtin: r.IsBuiltin, SupportsRDAP: r.SupportsRdap, Offsets: r.ReminderOffsets, AssetCount: r.AssetCount, Fields: byType[r.ID]}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Type, error) {
	r, err := s.q.GetAssetType(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Type{}, ErrNotFound
	}
	if err != nil {
		return Type{}, fmt.Errorf("get asset type: %w", err)
	}
	fields, err := s.Fields(ctx, id)
	if err != nil {
		return Type{}, err
	}
	n, err := s.q.CountAssetsByType(ctx, id)
	if err != nil {
		return Type{}, fmt.Errorf("count assets: %w", err)
	}
	return Type{ID: r.ID, Name: r.Name, Slug: r.Slug, Icon: r.Icon, Builtin: r.IsBuiltin, SupportsRDAP: r.SupportsRdap, Offsets: r.ReminderOffsets, AssetCount: n, Fields: fields}, nil
}

func (s *Service) Fields(ctx context.Context, typeID string) ([]Field, error) {
	rows, err := s.q.ListAssetTypeFields(ctx, typeID)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	out := make([]Field, len(rows))
	for i, r := range rows {
		out[i] = fieldFromRow(r)
	}
	return out, nil
}

type TypeInput struct {
	Name    string
	Icon    string
	Offsets []int32
}

func (s *Service) Create(ctx context.Context, in TypeInput) (string, error) {
	slug := Slugify(in.Name)
	if slug == "" {
		slug = "type"
	}
	base := slug
	for i := 2; ; i++ {
		_, err := s.q.GetAssetTypeBySlug(ctx, slug)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("check slug: %w", err)
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	t, err := s.q.CreateAssetType(ctx, sqlc.CreateAssetTypeParams{Name: strings.TrimSpace(in.Name), Slug: slug, Icon: in.Icon, ReminderOffsets: in.Offsets})
	if isUniqueViolation(err) {
		return "", ErrDuplicate
	}
	if err != nil {
		return "", fmt.Errorf("create asset type: %w", err)
	}
	return t.ID, nil
}

func (s *Service) Update(ctx context.Context, id string, in TypeInput) error {
	err := s.q.UpdateAssetType(ctx, sqlc.UpdateAssetTypeParams{ID: id, Name: strings.TrimSpace(in.Name), Icon: in.Icon, ReminderOffsets: in.Offsets})
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	t, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.Builtin {
		return ErrBuiltin
	}
	if t.AssetCount > 0 {
		return ErrInUse
	}
	return s.q.DeleteAssetType(ctx, id)
}

type FieldInput struct {
	Key      string
	Label    string
	Type     FieldType
	Options  []string
	Required bool
	Sort     int
}

func (s *Service) CreateField(ctx context.Context, typeID string, in FieldInput) error {
	opts, _ := json.Marshal(nonNil(in.Options))
	_, err := s.q.CreateAssetTypeField(ctx, sqlc.CreateAssetTypeFieldParams{
		AssetTypeID: typeID, Key: in.Key, Label: strings.TrimSpace(in.Label), FieldType: string(in.Type),
		Options: opts, IsRequired: in.Required, SortOrder: int32(in.Sort),
	})
	if isUniqueViolation(err) {
		return ErrDuplicate
	}
	return err
}

func (s *Service) Field(ctx context.Context, typeID, fieldID string) (Field, error) {
	r, err := s.q.GetAssetTypeField(ctx, fieldID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && r.AssetTypeID != typeID) {
		return Field{}, ErrFieldNotFound
	}
	if err != nil {
		return Field{}, fmt.Errorf("get field: %w", err)
	}
	return fieldFromRow(r), nil
}

func (s *Service) UpdateField(ctx context.Context, f Field, in FieldInput) error {
	opts, _ := json.Marshal(nonNil(in.Options))
	return s.q.UpdateAssetTypeField(ctx, sqlc.UpdateAssetTypeFieldParams{
		ID: f.ID, Label: strings.TrimSpace(in.Label), FieldType: string(in.Type), Options: opts, IsRequired: in.Required, SortOrder: int32(in.Sort),
	})
}

func (s *Service) CountFieldValues(ctx context.Context, f Field) (int64, error) {
	return s.q.CountAssetsWithFieldValue(ctx, sqlc.CountAssetsWithFieldValueParams{AssetTypeID: f.TypeID, Key: f.Key})
}

func (s *Service) RemoveField(ctx context.Context, f Field) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if err := q.RemoveFieldValues(ctx, sqlc.RemoveFieldValuesParams{AssetTypeID: f.TypeID, Key: f.Key}); err != nil {
		return fmt.Errorf("remove field values: %w", err)
	}
	if err := q.DeleteAssetTypeField(ctx, f.ID); err != nil {
		return fmt.Errorf("delete field: %w", err)
	}
	return tx.Commit(ctx)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
