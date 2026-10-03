package services

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func solidImage(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	return img
}

// withExifOrientation inserts an APP1 EXIF segment carrying the given
// orientation (plus a GPS IFD pointer) right after the JPEG SOI marker.
func withExifOrientation(jpg []byte, orientation uint16) []byte {
	var tiff bytes.Buffer
	tiff.WriteString("MM")
	binary.Write(&tiff, binary.BigEndian, uint16(42))
	binary.Write(&tiff, binary.BigEndian, uint32(8))
	binary.Write(&tiff, binary.BigEndian, uint16(2))
	// Orientation, SHORT, count 1.
	binary.Write(&tiff, binary.BigEndian, []uint16{0x0112, 3})
	binary.Write(&tiff, binary.BigEndian, uint32(1))
	binary.Write(&tiff, binary.BigEndian, []uint16{orientation, 0})
	// GPSInfo IFD pointer, LONG, count 1.
	binary.Write(&tiff, binary.BigEndian, []uint16{0x8825, 4})
	binary.Write(&tiff, binary.BigEndian, []uint32{1, 0})
	binary.Write(&tiff, binary.BigEndian, uint32(0))

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	segment := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))
	segment = append(segment, payload...)

	out := append([]byte{}, jpg[:2]...)
	out = append(out, segment...)
	return append(out, jpg[2:]...)
}

func TestProcessImageResizesAndStripsExif(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solidImage(3000, 2000), nil); err != nil {
		t.Fatal(err)
	}
	input := withExifOrientation(buf.Bytes(), 1)
	if !bytes.Contains(input, []byte("Exif")) {
		t.Fatal("test setup: EXIF not inserted")
	}

	out, err := ProcessImage(input)
	if err != nil {
		t.Fatal(err)
	}

	large, err := jpeg.DecodeConfig(bytes.NewReader(out.Large))
	if err != nil {
		t.Fatal(err)
	}
	if large.Width != LargeMaxEdge || large.Height != 1066 {
		t.Fatalf("large = %dx%d", large.Width, large.Height)
	}
	thumb, _ := jpeg.DecodeConfig(bytes.NewReader(out.Thumb))
	if thumb.Width != ThumbMaxEdge {
		t.Fatalf("thumb width = %d", thumb.Width)
	}
	if bytes.Contains(out.Large, []byte("Exif")) || bytes.Contains(out.Thumb, []byte("Exif")) {
		t.Fatal("output still contains EXIF")
	}
}

func TestProcessImageAppliesOrientation(t *testing.T) {
	var buf bytes.Buffer
	jpeg.Encode(&buf, solidImage(800, 400), nil)

	// Orientation 6 means the camera was rotated 90°; output must be portrait.
	out, err := ProcessImage(withExifOrientation(buf.Bytes(), 6))
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out.Large))
	if cfg.Width != 400 || cfg.Height != 800 {
		t.Fatalf("oriented = %dx%d, want 400x800", cfg.Width, cfg.Height)
	}
}

func TestProcessImageAcceptsPNG(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, solidImage(100, 50))

	out, err := ProcessImage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if SniffImageType(out.Large) != "image/jpeg" {
		t.Fatal("expected JPEG output")
	}
}

func TestProcessImageRejectsNonImages(t *testing.T) {
	inputs := [][]byte{
		[]byte("%PDF-1.7 not a photo"),
		[]byte("MZ\x90\x00 executable"),
		append([]byte{0xFF, 0xD8, 0xFF}, []byte("truncated jpeg")...),
	}
	for _, input := range inputs {
		if _, err := ProcessImage(input); !errors.Is(err, ErrUnsupportedImage) {
			t.Errorf("expected ErrUnsupportedImage for %q, got %v", input[:4], err)
		}
	}
}

func TestProcessImageRejectsDecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1)))
	data := buf.Bytes()
	// Patch the IHDR dimensions to 20000x20000 without changing pixel data.
	binary.BigEndian.PutUint32(data[16:], 20000)
	binary.BigEndian.PutUint32(data[20:], 20000)

	if _, err := ProcessImage(data); !errors.Is(err, ErrUnsupportedImage) {
		t.Fatalf("expected rejection, got %v", err)
	}
}
