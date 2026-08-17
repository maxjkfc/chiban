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
	// MaxSourceDimension rejects absurd images outright rather than spending
	// memory decoding them; a decompression bomb never reaches the resizer.
	MaxSourceDimension = 10000
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

// Sanitize validates data as an image and returns what should be stored.
//
// The declared filename and Content-Type are ignored: only what actually
// decodes counts.
func Sanitize(data []byte) (Sanitized, error) {
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
	if config.Width > MaxSourceDimension || config.Height > MaxSourceDimension {
		return Sanitized{}, InvalidInputError{
			Message: fmt.Sprintf("image is larger than %dx%d", MaxSourceDimension, MaxSourceDimension),
		}
	}

	if format == "gif" {
		return sanitizeGIF(data)
	}
	return sanitizeStill(data)
}

// sanitizeGIF keeps the original bytes so animation survives. It still decodes
// every frame first, so a file that merely claims to be a GIF is rejected.
func sanitizeGIF(data []byte) (Sanitized, error) {
	decoded, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a valid GIF"}
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
