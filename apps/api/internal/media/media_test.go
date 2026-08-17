package media_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
)

// Sanitisation is one of the few things the spec allows a unit test for: it is
// a pure input/output transform with more edge cases than it is worth
// enumerating through HTTP.

func TestRejectsFilesThatAreNotImages(t *testing.T) {
	cases := map[string][]byte{
		"empty":                  {},
		"plain text":             []byte("this is not an image at all"),
		"jpeg header only":       {0xFF, 0xD8, 0xFF, 0xE0},
		"gif magic without data": []byte("GIF89a"),
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := media.Sanitize(data); err == nil {
				t.Fatal("accepted a file that is not a decodable image")
			}
		})
	}
}

// A .jpg extension proves nothing; only what decodes counts.
func TestRejectsTextDisguisedAsAnImage(t *testing.T) {
	data := []byte(strings.Repeat("definitely not pixels", 100))

	if _, err := media.Sanitize(data); err == nil {
		t.Fatal("accepted text as an image")
	}
}

func TestRejectsFilesOverTheSizeLimit(t *testing.T) {
	oversized := make([]byte, media.MaxUploadBytes+1)

	_, err := media.Sanitize(oversized)

	var invalid media.InvalidInputError
	if !asInvalid(err, &invalid) {
		t.Fatalf("err = %v, want an InvalidInputError about size", err)
	}
}

// Phone photos are far bigger than any screen needs, so they are scaled down
// rather than rejected — rejecting them would make the app unusable.
func TestLargePhotosAreScaledDown(t *testing.T) {
	source := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 4032, 3024)))

	out, err := media.Sanitize(source)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Width > media.MaxStoredDimension || out.Height > media.MaxStoredDimension {
		t.Fatalf("stored size = %dx%d, want both within %d",
			out.Width, out.Height, media.MaxStoredDimension)
	}
	if out.Width != media.MaxStoredDimension {
		t.Fatalf("width = %d, want the long edge scaled to %d", out.Width, media.MaxStoredDimension)
	}
	if len(out.Data) >= len(source) {
		t.Fatal("scaled image is not smaller than the original")
	}
}

func TestSmallImagesKeepTheirSize(t *testing.T) {
	source := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 800, 600)))

	out, err := media.Sanitize(source)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Width != 800 || out.Height != 600 {
		t.Fatalf("size = %dx%d, want 800x600", out.Width, out.Height)
	}
}

// The whole point of re-encoding: a photo carrying GPS coordinates must not
// keep them once stored, or sharing a meal would share where it was eaten.
func TestExifAndGPSMetadataAreRemoved(t *testing.T) {
	source := jpegWithGPSExif(t)

	if !bytes.Contains(source, []byte("Exif")) {
		t.Fatal("test fixture has no EXIF block to strip")
	}

	out, err := media.Sanitize(source)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if bytes.Contains(out.Data, []byte("Exif")) {
		t.Fatal("stored image still contains an EXIF block")
	}
	if bytes.Contains(out.Data, []byte{0xFF, 0xE1}) {
		t.Fatal("stored image still contains an APP1 marker")
	}
}

// GIFs must keep every frame: flattening one into a still image would quietly
// break the reaction the user was sending.
func TestAnimatedGIFsKeepEveryFrame(t *testing.T) {
	source := encodeAnimatedGIF(t, 3)

	out, err := media.Sanitize(source)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Kind != media.KindGIF {
		t.Fatalf("kind = %q, want %q", out.Kind, media.KindGIF)
	}
	if out.ContentType != "image/gif" {
		t.Fatalf("content type = %q, want image/gif", out.ContentType)
	}

	decoded, err := gif.DecodeAll(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatalf("stored GIF does not decode: %v", err)
	}
	if len(decoded.Image) != 3 {
		t.Fatalf("stored GIF has %d frames, want 3", len(decoded.Image))
	}
}

func TestPNGIsAcceptedAndStoredAsJPEG(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	out, err := media.Sanitize(buf.Bytes())
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Kind != media.KindImage || out.ContentType != "image/jpeg" {
		t.Fatalf("out = %q / %q, want image / image/jpeg", out.Kind, out.ContentType)
	}
	if out.Extension() != "jpg" {
		t.Fatalf("extension = %q, want jpg", out.Extension())
	}
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodeAnimatedGIF(t *testing.T, frames int) []byte {
	t.Helper()

	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{}
	for i := 0; i < frames; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, 20, 20), palette)
		frame.SetColorIndex(i, i, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animation); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

// jpegWithGPSExif builds a JPEG carrying a minimal but real APP1/EXIF block
// with a GPS IFD, so the stripping test has something genuine to remove.
func jpegWithGPSExif(t *testing.T) []byte {
	t.Helper()

	base := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 40, 40)))

	exif := buildGPSExif()
	app1 := []byte{0xFF, 0xE1}
	length := len(exif) + 2
	app1 = append(app1, byte(length>>8), byte(length))
	app1 = append(app1, exif...)

	// Insert the APP1 segment straight after the SOI marker.
	out := make([]byte, 0, len(base)+len(app1))
	out = append(out, base[:2]...)
	out = append(out, app1...)
	out = append(out, base[2:]...)
	return out
}

func buildGPSExif() []byte {
	var b bytes.Buffer
	b.WriteString("Exif\x00\x00")
	b.WriteString("MM\x00\x2a")   // big-endian TIFF header
	b.Write([]byte{0, 0, 0, 8})   // offset of the first IFD
	b.Write([]byte{0, 1})         // one entry
	b.Write([]byte{0x88, 0x25})   // GPSInfo IFD pointer tag
	b.Write([]byte{0, 4})         // type LONG
	b.Write([]byte{0, 0, 0, 1})   // one value
	b.Write([]byte{0, 0, 0, 26})  // offset of the GPS IFD
	b.Write([]byte{0, 0, 0, 0})   // no next IFD
	b.Write([]byte{0, 1})         // GPS IFD: one entry
	b.Write([]byte{0, 1})         // GPSLatitudeRef
	b.Write([]byte{0, 2})         // type ASCII
	b.Write([]byte{0, 0, 0, 2})   // two bytes
	b.Write([]byte{'N', 0, 0, 0}) // "N"
	b.Write([]byte{0, 0, 0, 0})   // no next IFD
	return b.Bytes()
}

func asInvalid(err error, target *media.InvalidInputError) bool {
	invalid, ok := err.(media.InvalidInputError)
	if ok {
		*target = invalid
	}
	return ok
}
