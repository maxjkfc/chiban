package media_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
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

	out, err := media.SanitizeAllowingGIF(source)
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

// A phone set to High Efficiency — the iPhone default — hands the browser a
// HEIC, which nothing outside Apple's platforms renders. Accepting it and
// storing the JPEG is what makes "take a photo, publish" work at all.
func TestHEICFromAPhoneIsAcceptedAndStoredAsJPEG(t *testing.T) {
	out, err := media.Sanitize(readFixture(t, "portrait.heic"))
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Kind != media.KindImage || out.ContentType != "image/jpeg" {
		t.Fatalf("out = %q / %q, want image / image/jpeg", out.Kind, out.ContentType)
	}
	if out.Extension() != "jpg" {
		t.Fatalf("extension = %q, want jpg", out.Extension())
	}
	if _, err := jpeg.Decode(bytes.NewReader(out.Data)); err != nil {
		t.Fatalf("stored image does not decode as JPEG: %v", err)
	}
	if out.Width != 40 || out.Height != 60 {
		t.Fatalf("stored %dx%d, want the source's 40x60", out.Width, out.Height)
	}
}

// HEIC carries the camera's rotation beside the pixels rather than in them, so
// a decoder that ignores it stores every portrait photo on its side. The
// fixture is the same 40x60 canvas tagged as rotated a quarter turn.
func TestHEICRotationIsApplied(t *testing.T) {
	out, err := media.Sanitize(readFixture(t, "portrait-rotated.heic"))
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}

	if out.Width != 60 || out.Height != 40 {
		t.Fatalf("stored %dx%d, want 60x40 — the rotation was not applied", out.Width, out.Height)
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
	return encodeAnimatedGIFOfSize(t, frames, 20)
}

func encodeAnimatedGIFOfSize(t *testing.T, frames, size int) []byte {
	t.Helper()

	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{}
	for i := 0; i < frames; i++ {
		frame := image.NewPaletted(image.Rect(0, 0, size, size), palette)
		frame.SetColorIndex(i%size, i%size, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animation); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
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

// A meal photo is a photograph. Accepting an animation there would mean the
// recording flow silently runs the GIF decoder, which nothing asks for.
func TestSanitizeRejectsGIFButSanitizeAllowingGIFAcceptsIt(t *testing.T) {
	source := encodeAnimatedGIF(t, 2)

	if _, err := media.Sanitize(source); err == nil {
		t.Fatal("Sanitize accepted a GIF")
	}
	if _, err := media.SanitizeAllowingGIF(source); err != nil {
		t.Fatalf("SanitizeAllowingGIF rejected a GIF: %v", err)
	}
}

// Importing an image library registers its formats globally. TIFF and BMP
// arrive that way, and accepting them would run decoders this product never
// chose to depend on.
func TestRejectsFormatsThatWereOnlyRegisteredTransitively(t *testing.T) {
	// A minimal little-endian TIFF: header, one IFD, one 1x1 strip.
	tiff := []byte{
		'I', 'I', 42, 0, 8, 0, 0, 0,
		1, 0,
		0x00, 0x01, 3, 0, 1, 0, 0, 0, 1, 0, 0, 0,
		0, 0, 0, 0,
	}

	if _, err := media.Sanitize(tiff); err == nil {
		t.Fatal("accepted a TIFF; only JPEG and PNG are supported")
	}
}

// Dimensions bound the shape, not the memory: this is the check that stops a
// well-compressed image from decoding into hundreds of megabytes.
func TestRejectsImagesOverTheDecodedPixelBudget(t *testing.T) {
	// 9000x9000 is within MaxSourceDimension but far past the pixel budget.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 9000, 9000))); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	_, err := media.Sanitize(buf.Bytes())

	var invalid media.InvalidInputError
	if !asInvalid(err, &invalid) {
		t.Fatalf("err = %v, want an InvalidInputError about megapixels", err)
	}
}

// The bomb john reproduced: thousands of tiny frames compress to a couple of
// megabytes and decode to hundreds.
func TestRejectsGIFsWithTooManyFramesForTheirSize(t *testing.T) {
	// 20000 frames of 50x50 decode to 50 million pixels, well past the budget,
	// from a file small enough that the size limit never fires.
	source := encodeAnimatedGIFOfSize(t, 20000, 50)

	if len(source) > media.MaxUploadBytes {
		t.Fatalf("fixture is %d bytes, which the size limit would catch first", len(source))
	}

	if _, err := media.SanitizeAllowingGIF(source); err == nil {
		t.Fatal("accepted a GIF whose frames decode far past the pixel budget")
	}
}
