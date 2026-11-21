package document

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/suwahadi/project-ops-manager/internal/db/sqlc"
	"github.com/suwahadi/project-ops-manager/internal/imaging"
)

const MaxFileSize = 20 << 20

var (
	ErrTooLarge    = errors.New("file is larger than 20 MB")
	ErrUnsupported = errors.New("only PDF, JPEG, PNG and WebP files are accepted")
	ErrEmpty       = errors.New("file is empty")
	ErrNotFound    = errors.New("document not found")
)

type Upload struct {
	Name string
	Data []byte
}

type Prepared struct {
	OriginalName string
	StoredPath   string
	ContentType  string
	Size         int64
}

type Store struct {
	root string
	q    *sqlc.Queries
}

func NewStore(root string, db sqlc.DBTX) *Store {
	return &Store{root: root, q: sqlc.New(db)}
}

func Sniff(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrEmpty
	}
	ct := http.DetectContentType(data)
	switch {
	case ct == "application/pdf":
		return imaging.ContentPDF, nil
	case ct == "image/jpeg", ct == "image/png", ct == "image/webp":
		return ct, nil
	}
	return "", ErrUnsupported
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func CleanName(name string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		name = "document"
	}
	if len(name) > 200 {
		name = name[len(name)-200:]
	}
	return name
}

func (s *Store) Prepare(up Upload, now time.Time) (Prepared, error) {
	if len(up.Data) > MaxFileSize {
		return Prepared{}, ErrTooLarge
	}
	ct, err := Sniff(up.Data)
	if err != nil {
		return Prepared{}, err
	}
	data := up.Data
	name := CleanName(up.Name)
	ext := "pdf"
	if ct != imaging.ContentPDF {
		data, err = imaging.ToWebP(up.Data, ct)
		if err != nil {
			return Prepared{}, fmt.Errorf("convert image: %w", err)
		}
		ct = imaging.ContentWebP
		ext = "webp"
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".webp"
	}
	id, err := newUUID()
	if err != nil {
		return Prepared{}, err
	}
	rel := fmt.Sprintf("%04d/%02d/%s.%s", now.Year(), int(now.Month()), id, ext)
	if err := s.write(rel, data); err != nil {
		return Prepared{}, err
	}
	return Prepared{OriginalName: name, StoredPath: rel, ContentType: ct, Size: int64(len(data))}, nil
}

func (s *Store) write(rel string, data []byte) error {
	full := s.Path(rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return fmt.Errorf("create upload directory: %w", err)
	}
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("write upload: %w", err)
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("store upload: %w", err)
	}
	return nil
}

func (s *Store) WriteFile(rel string, data []byte) error { return s.write(rel, data) }

func (s *Store) Path(rel string) string {
	return filepath.Join(s.root, filepath.Clean("/"+rel))
}

func (s *Store) Remove(rels ...string) {
	for _, r := range rels {
		if r == "" {
			continue
		}
		_ = os.Remove(s.Path(r))
	}
}

func (s *Store) RemovePrepared(p []Prepared) {
	for _, x := range p {
		s.Remove(x.StoredPath)
	}
}

func Insert(ctx context.Context, q *sqlc.Queries, assetID, renewalID *string, p Prepared) error {
	_, err := q.CreateDocument(ctx, sqlc.CreateDocumentParams{
		AssetID: assetID, RenewalID: renewalID, OriginalName: p.OriginalName,
		StoredPath: p.StoredPath, ContentType: p.ContentType, SizeBytes: p.Size,
	})
	if err != nil {
		return fmt.Errorf("insert document: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, id string) (sqlc.GetDocumentRow, error) {
	d, err := s.q.GetDocument(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.q.DeleteDocument(ctx, id); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	s.Remove(d.StoredPath)
	return nil
}

func (s *Store) ListForAsset(ctx context.Context, assetID string) ([]sqlc.Document, []sqlc.Document, error) {
	own, err := s.q.ListAssetDocuments(ctx, &assetID)
	if err != nil {
		return nil, nil, fmt.Errorf("list asset documents: %w", err)
	}
	ren, err := s.q.ListRenewalDocumentsByAsset(ctx, assetID)
	if err != nil {
		return nil, nil, fmt.Errorf("list renewal documents: %w", err)
	}
	return own, ren, nil
}
