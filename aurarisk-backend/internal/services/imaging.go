package services

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	LargeMaxEdge   = 1600
	ThumbMaxEdge   = 400
	maxImagePixels = 50_000_000
	jpegQuality    = 82
)

var AllowedPhotoTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// ErrUnsupportedImage marks input that is not a decodable photo. Callers
// treat it as a permanent rejection rather than a retryable failure.
var ErrUnsupportedImage = errors.New("unsupported image")

type ProcessedImage struct {
	Large []byte
	Thumb []byte
}

// SniffImageType returns the content type detected from magic bytes.
func SniffImageType(data []byte) string {
	return http.DetectContentType(data)
}

// ProcessImage validates, orients, and re-encodes an uploaded photo. The
// output is a fresh JPEG with no EXIF or other metadata.
func ProcessImage(data []byte) (*ProcessedImage, error) {
	if !AllowedPhotoTypes[SniffImageType(data)] {
		return nil, fmt.Errorf("%w: content type %s", ErrUnsupportedImage, SniffImageType(data))
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, fmt.Errorf("%w: dimensions %dx%d", ErrUnsupportedImage, cfg.Width, cfg.Height)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedImage, err)
	}
	src = applyOrientation(src, jpegOrientation(data))

	large, err := encodeResized(src, LargeMaxEdge)
	if err != nil {
		return nil, err
	}
	thumb, err := encodeResized(src, ThumbMaxEdge)
	if err != nil {
		return nil, err
	}
	return &ProcessedImage{Large: large, Thumb: thumb}, nil
}

func encodeResized(src image.Image, maxEdge int) ([]byte, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > maxEdge || h > maxEdge {
		if w >= h {
			h = h * maxEdge / w
			w = maxEdge
		} else {
			w = w * maxEdge / h
			h = maxEdge
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill white so transparent PNG/WebP regions don't become black in JPEG.
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)

	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// jpegOrientation reads the EXIF Orientation tag (1-8) from a JPEG, returning
// 1 when absent or unreadable.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	pos := 2
	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			return 1
		}
		marker := data[pos+1]
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(data[pos+2:]))
		if segLen < 2 || pos+2+segLen > len(data) {
			return 1
		}
		segment := data[pos+4 : pos+2+segLen]
		if marker == 0xE1 && len(segment) > 6 && string(segment[:6]) == "Exif\x00\x00" {
			return exifOrientation(segment[6:])
		}
		pos += 2 + segLen
	}
	return 1
}

func exifOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	entries := int(order.Uint16(tiff[ifd:]))
	for i := 0; i < entries; i++ {
		entry := ifd + 2 + i*12
		if entry+12 > len(tiff) {
			return 1
		}
		if order.Uint16(tiff[entry:]) == 0x0112 {
			value := int(order.Uint16(tiff[entry+8:]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
	}
	return 1
}

// applyOrientation returns src transformed so it displays upright.
func applyOrientation(src image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := orientation >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
