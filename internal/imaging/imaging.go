package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"

	"github.com/HugoSmits86/nativewebp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	MaxSide     = 2560
	maxPixels   = 60_000_000
	ContentWebP = "image/webp"
	ContentJPEG = "image/jpeg"
	ContentPNG  = "image/png"
	ContentPDF  = "application/pdf"
)

var ErrTooLarge = errors.New("image dimensions are too large")

func decode(data []byte, contentType string) (image.Image, error) {
	var cfg image.Config
	var err error
	switch contentType {
	case ContentJPEG:
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	case ContentPNG:
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	case ContentWebP:
		cfg, err = webp.DecodeConfig(bytes.NewReader(data))
	default:
		return nil, fmt.Errorf("unsupported image type %q", contentType)
	}
	if err != nil {
		return nil, fmt.Errorf("read image header: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, ErrTooLarge
	}
	var img image.Image
	switch contentType {
	case ContentJPEG:
		img, err = jpeg.Decode(bytes.NewReader(data))
	case ContentPNG:
		img, err = png.Decode(bytes.NewReader(data))
	default:
		img, err = webp.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

func ToWebP(data []byte, contentType string) ([]byte, error) {
	img, err := decode(data, contentType)
	if err != nil {
		return nil, err
	}
	if contentType == ContentJPEG {
		img = applyOrientation(img, jpegOrientation(data))
	}
	img = fit(img, MaxSide)
	var out bytes.Buffer
	if err := nativewebp.Encode(&out, toNRGBA(img), &nativewebp.Options{CompressionLevel: nativewebp.DefaultCompression}); err != nil {
		return nil, fmt.Errorf("encode webp: %w", err)
	}
	return out.Bytes(), nil
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func fit(img image.Image, maxSide int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSide && h <= maxSide {
		return img
	}
	var nw, nh int
	if w >= h {
		nw, nh = maxSide, int(float64(h)*float64(maxSide)/float64(w)+0.5)
	} else {
		nw, nh = int(float64(w)*float64(maxSide)/float64(h)+0.5), maxSide
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}
