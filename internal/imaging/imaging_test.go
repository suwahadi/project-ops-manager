package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

func sample(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	return img
}

func decodeWebP(t *testing.T, b []byte) image.Image {
	t.Helper()
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		t.Fatal("output is not a WebP file")
	}
	img, err := webp.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	return img
}

func TestPNGToWebP(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, sample(40, 20)); err != nil {
		t.Fatal(err)
	}
	out, err := ToWebP(buf.Bytes(), ContentPNG)
	if err != nil {
		t.Fatal(err)
	}
	img := decodeWebP(t, out)
	if img.Bounds().Dx() != 40 || img.Bounds().Dy() != 20 {
		t.Fatalf("size = %v", img.Bounds())
	}
	r, g, _, _ := img.At(10, 5).RGBA()
	if r>>8 != 10 || g>>8 != 5 {
		t.Fatalf("lossless pixel mismatch: %d %d", r>>8, g>>8)
	}
}

func TestJPEGResizedAndMetadataStripped(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, sample(3000, 1500), &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	withExif := insertExif(buf.Bytes(), 1)
	out, err := ToWebP(withExif, ContentJPEG)
	if err != nil {
		t.Fatal(err)
	}
	img := decodeWebP(t, out)
	if img.Bounds().Dx() != MaxSide || img.Bounds().Dy() != 1280 {
		t.Fatalf("expected longest side capped at %d, got %v", MaxSide, img.Bounds())
	}
	if bytes.Contains(out, []byte("Exif")) {
		t.Fatal("metadata was not stripped")
	}
}

func TestWebPReencodedAndOrientationApplied(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, sample(30, 10), nil); err != nil {
		t.Fatal(err)
	}
	out, err := ToWebP(insertExif(buf.Bytes(), 6), ContentJPEG)
	if err != nil {
		t.Fatal(err)
	}
	img := decodeWebP(t, out)
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 30 {
		t.Fatalf("orientation 6 should rotate to portrait, got %v", img.Bounds())
	}
	again, err := ToWebP(out, ContentWebP)
	if err != nil {
		t.Fatal(err)
	}
	decodeWebP(t, again)
}

func TestRejectsGarbage(t *testing.T) {
	if _, err := ToWebP([]byte("not an image"), ContentPNG); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ToWebP([]byte("%PDF-1.7"), ContentPDF); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func insertExif(jpg []byte, orientation uint16) []byte {
	tiff := make([]byte, 0, 32)
	tiff = append(tiff, 'M', 'M', 0, 42, 0, 0, 0, 8)
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112)
	tiff = binary.BigEndian.AppendUint16(tiff, 3)
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, orientation)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1}
	seg = binary.BigEndian.AppendUint16(seg, uint16(len(payload)+2))
	seg = append(seg, payload...)
	out := append([]byte{}, jpg[:2]...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}
