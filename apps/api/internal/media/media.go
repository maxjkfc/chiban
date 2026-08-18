// Package media validates and normalises uploaded images.
//
// Every upload in the product — meal photos, chat images, stickers, avatars —
// goes through Sanitize. Nothing is stored as the client sent it: static
// images are decoded and re-encoded, which is also what removes EXIF and GPS
// metadata, and animated GIFs take a separate path that keeps every frame.
package media

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	// Registers the PNG decoder with image.DecodeConfig; jpeg and gif are
	// registered by the direct imports above.
	_ "image/png"

	"github.com/disintegration/imaging"
)

// InvalidInputError describes an upload the user can fix.
type InvalidInputError struct {
	Message string
}

func (e InvalidInputError) Error() string { return "media: " + e.Message }

const (
	// MaxUploadBytes is the largest upload accepted, before decoding.
	MaxUploadBytes = 10 << 20 // 10 MiB
	// MaxSourceDimension rejects absurd shapes outright.
	MaxSourceDimension = 10000
	// MaxDecodedPixels bounds what decoding will actually allocate. File size
	// says nothing about that: a well-compressed 10 MiB image can decode to
	// hundreds of megabytes. Roughly 31 MP — comfortably above any phone
	// camera, far below what would threaten the Mac mini.
	MaxDecodedPixels = 30 << 20
	// MaxStoredDimension is the longest edge kept after resizing. Phone photos
	// are far larger than a mobile screen ever needs.
	MaxStoredDimension = 1600
	jpegQuality        = 85
)

// Kind distinguishes the two storage paths. Animated GIFs must not be flattened
// into a still image, so they never go through the re-encoder.
type Kind string

const (
	KindImage Kind = "image"
	KindGIF   Kind = "gif"
)

type Sanitized struct {
	Data        []byte
	ContentType string
	Kind        Kind
	Width       int
	Height      int
}

// Sanitize accepts a still photo — JPEG or PNG — and returns what to store.
//
// The declared filename and Content-Type are ignored: only what actually
// decodes counts. Animated GIFs are rejected here; chat media and stickers use
// SanitizeAllowingGIF, because a meal photo is a photograph and nothing in the
// recording flow sends an animation.
func Sanitize(data []byte) (Sanitized, error) {
	return sanitize(data, false)
}

// SanitizeAllowingGIF also accepts animated GIFs, keeping every frame.
func SanitizeAllowingGIF(data []byte) (Sanitized, error) {
	return sanitize(data, true)
}

func sanitize(data []byte, allowGIF bool) (Sanitized, error) {
	if len(data) == 0 {
		return Sanitized{}, InvalidInputError{Message: "file is empty"}
	}
	if len(data) > MaxUploadBytes {
		return Sanitized{}, InvalidInputError{
			Message: fmt.Sprintf("file is larger than %d MB", MaxUploadBytes>>20),
		}
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a supported image"}
	}
	// Whitelist by what actually decoded, not by what happens to be registered.
	// Importing an image library registers its own formats globally — TIFF and
	// BMP arrive that way — and silently accepting them would run decoders
	// this product never chose to depend on.
	switch {
	case format == "jpeg" || format == "png":
	case format == "gif" && allowGIF:
	default:
		return Sanitized{}, InvalidInputError{Message: "file must be a JPEG or PNG image"}
	}

	if config.Width > MaxSourceDimension || config.Height > MaxSourceDimension {
		return Sanitized{}, InvalidInputError{
			Message: fmt.Sprintf("image is larger than %dx%d", MaxSourceDimension, MaxSourceDimension),
		}
	}
	if config.Width*config.Height > MaxDecodedPixels {
		return Sanitized{}, InvalidInputError{
			Message: fmt.Sprintf("image has more than %d megapixels", MaxDecodedPixels>>20),
		}
	}

	if format == "gif" {
		return sanitizeGIF(data)
	}
	return sanitizeStill(data)
}

// sanitizeGIF keeps the original bytes so animation survives. It still decodes
// every frame first, so a file that merely claims to be a GIF is rejected.
//
// ponytail: the frame budget is checked after DecodeAll, so it bounds what is
// stored but not the peak allocation while decoding — stdlib has no streaming
// GIF decoder to stop earlier. Acceptable while nothing reaches this path:
// Sanitize rejects GIFs, so only SanitizeAllowingGIF does, and its first
// caller arrives with chat media in slice 10. Bound the spike properly there.
func sanitizeGIF(data []byte) (Sanitized, error) {
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a valid GIF"}
	}

	// Frame count multiplies the canvas: a 2 MB file of thousands of small
	// frames decodes to hundreds of megabytes, which neither the size nor the
	// dimension limit catches.
	if frames := len(decoded.Image); frames*decoded.Config.Width*decoded.Config.Height > MaxDecodedPixels {
		return Sanitized{}, InvalidInputError{Message: "animation has too many frames for its size"}
	}

	return Sanitized{
		Data:        data,
		ContentType: "image/gif",
		Kind:        KindGIF,
		Width:       decoded.Config.Width,
		Height:      decoded.Config.Height,
	}, nil
}

// sanitizeStill decodes, applies the EXIF orientation and re-encodes as JPEG.
//
// Re-encoding from the decoded pixels is what strips EXIF and GPS: none of the
// original metadata survives the round trip. Applying the orientation first
// matters because dropping the tag without rotating would leave every portrait
// phone photo lying on its side.
func sanitizeStill(data []byte) (Sanitized, error) {
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a supported image"}
	}

	bounds := img.Bounds()
	if bounds.Dx() > MaxStoredDimension || bounds.Dy() > MaxStoredDimension {
		img = imaging.Fit(img, MaxStoredDimension, MaxStoredDimension, imaging.Lanczos)
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return Sanitized{}, fmt.Errorf("media: encode: %w", err)
	}

	return Sanitized{
		Data:        out.Bytes(),
		ContentType: "image/jpeg",
		Kind:        KindImage,
		Width:       img.Bounds().Dx(),
		Height:      img.Bounds().Dy(),
	}, nil
}

// Extension is the file extension for a sanitized object.
func (s Sanitized) Extension() string {
	if s.Kind == KindGIF {
		return "gif"
	}
	return "jpg"
}
