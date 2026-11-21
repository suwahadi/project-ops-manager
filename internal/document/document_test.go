package document

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPrepare(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, nil)
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

	pdf := []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\ntrailer\n%%EOF\n")
	p, err := s.Prepare(Upload{Name: "../../invoice.png", Data: pdf}, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.ContentType != "application/pdf" || !strings.HasPrefix(p.StoredPath, "2026/11/") || !strings.HasSuffix(p.StoredPath, ".pdf") || p.OriginalName != "invoice.png" {
		t.Fatalf("pdf prepared = %+v", p)
	}
	stored, err := os.ReadFile(s.Path(p.StoredPath))
	if err != nil || !bytes.Equal(stored, pdf) {
		t.Fatal("pdf must be stored unchanged")
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	img, err := s.Prepare(Upload{Name: "receipt.pdf", Data: buf.Bytes()}, now)
	if err != nil {
		t.Fatal(err)
	}
	if img.ContentType != "image/webp" || !strings.HasSuffix(img.StoredPath, ".webp") || img.OriginalName != "receipt.webp" {
		t.Fatalf("sniffing must ignore the extension: %+v", img)
	}

	if _, err := s.Prepare(Upload{Name: "x.pdf", Data: []byte("<html>not a pdf</html>")}, now); err != ErrUnsupported {
		t.Fatalf("expected unsupported, got %v", err)
	}
	if _, err := s.Prepare(Upload{Name: "big.pdf", Data: make([]byte, MaxFileSize+1)}, now); err != ErrTooLarge {
		t.Fatalf("expected too large, got %v", err)
	}
	if s.Path("../../etc/passwd") != dir+"/etc/passwd" {
		t.Fatalf("path traversal not contained: %s", s.Path("../../etc/passwd"))
	}
}
