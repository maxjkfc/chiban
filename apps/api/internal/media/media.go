// Package media validates and normalises uploaded images.
//
// Every upload in the product — meal photos, chat images, stickers, avatars —
// goes through Sanitize. Nothing is stored as the client sent it: still images
// are decoded and re-encoded as JPEG, which is also what removes EXIF and GPS
// metadata and what makes an iPhone's HEIC readable by every browser, and
// animated GIFs take a separate path that keeps every frame.

package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	// Registers the PNG decoder with image.DecodeConfig; jpeg and gif are
	// registered by the direct imports above.
	_ "image/png"

	"github.com/disintegration/imaging"
	"github.com/gen2brain/heic"
)

// A phone shooting in High Efficiency hands Safari the HEIC straight out of
// the camera roll, so refusing the format would mean refusing the default
// iPhone photo. The decoder is a Rust HEVC implementation compiled to WASM:
// no cgo, which the static distroless build requires, and untrusted frames
// are decoded inside the sandbox rather than by a C library in-process.
//
// The package would otherwise prefer a system libheif when one happens to be
// installed. Forcing WASM keeps a developer's Homebrew libheif from being a
// different decoder than production's, where none exists.
func init() {
	heic.ForceWasmMode = true
}

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
	// MaxGIFFrames bounds frame count on its own, independent of canvas size.
	//
	// The pixel budget alone is not enough: it allows more frames the smaller
	// the canvas gets, but each frame costs a fixed amount regardless of size
	// — an image.Paletted header, its palette, the LZW reader's state. Measured
	// at roughly 21 KB a frame, so a 4x4 canvas with 250,000 frames fits in a
	// 7.5 MB file, passes a pixels-only budget, and allocates 5.2 GB.
	//
	// A thousand is far past any real animation: reaction GIFs run tens of
	// frames. Together with the pixel budget the worst accepted case is about
	// 50 MB — 1000 frames of fixed cost plus the 30 MP of pixels.
	MaxGIFFrames = 1000
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

// Sanitize accepts a still photo — JPEG, PNG or HEIC — and returns what to
// store.
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
	case format == "jpeg" || format == "png" || format == "heic":
	case format == "gif" && allowGIF:
	default:
		return Sanitized{}, InvalidInputError{Message: "file must be a JPEG, PNG or HEIC image"}
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

// sanitizeGIF keeps the original bytes so animation survives.
//
// The frame budget is checked before decoding, not after: gif.DecodeAll
// allocates every frame up front, so a file of thousands of tiny frames on a
// large canvas costs hundreds of megabytes before any post-hoc check could
// run. Counting frames from the block structure is a scan with no pixel
// decoding at all, which is what makes the bound real rather than advisory.
//
// The file is still decoded afterwards, so something that merely claims to be
// a GIF is rejected rather than stored.
func sanitizeGIF(data []byte) (Sanitized, error) {
	// DecodeConfig reads the header and the global colour table; it does not
	// touch frame data, so it is safe to ask before the budget is known.
	config, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a valid GIF"}
	}

	canvas := config.Width * config.Height
	if canvas <= 0 {
		return Sanitized{}, InvalidInputError{Message: "file is not a valid GIF"}
	}

	// Frame count multiplies the canvas: a 2 MB file of thousands of small
	// frames decodes to hundreds of megabytes, which neither the size nor the
	// dimension limit catches.
	// Two bounds, because there are two shapes of bomb: a few frames on a huge
	// canvas, and a great many frames on a tiny one. Neither bound catches the
	// other's shape.
	maxFrames := min(MaxGIFFrames, MaxDecodedPixels/canvas)
	frames, err := gifFrameCount(data, maxFrames)
	if err != nil {
		return Sanitized{}, InvalidInputError{Message: "file is not a valid GIF"}
	}
	if frames > maxFrames {
		return Sanitized{}, InvalidInputError{Message: "animation has too many frames for its size"}
	}

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

// errMalformedGIF ends a scan that ran off the end of the data or hit a block
// type GIF does not define.
var errMalformedGIF = errors.New("media: malformed GIF")

// gifFrameCount walks a GIF's block structure and counts image descriptors.
//
// Nothing is decoded: colour tables and compressed pixel data are stepped over
// by their declared lengths. It stops as soon as the count passes limit, so a
// decompression bomb costs a walk of the file rather than a decode of it. The
// returned count is therefore only exact up to limit+1, which is all the
// caller needs to decide.
func gifFrameCount(data []byte, limit int) (int, error) {
	// Header (6) plus logical screen descriptor (7).
	const headerLen = 13
	if len(data) < headerLen {
		return 0, errMalformedGIF
	}

	at := headerLen
	if packed := data[10]; packed&0x80 != 0 {
		at += colourTableLen(packed)
	}

	frames := 0
	for {
		if at >= len(data) {
			return 0, errMalformedGIF
		}

		switch data[at] {
		case 0x3B: // trailer
			return frames, nil

		case 0x21: // extension: introducer, label, then data sub-blocks
			var err error
			if at, err = skipSubBlocks(data, at+2); err != nil {
				return 0, err
			}

		case 0x2C: // image descriptor: separator plus nine bytes
			frames++
			if frames > limit {
				return frames, nil
			}
			if at+10 > len(data) {
				return 0, errMalformedGIF
			}
			local := data[at+9]
			at += 10
			if local&0x80 != 0 {
				at += colourTableLen(local)
			}
			at++ // LZW minimum code size

			var err error
			if at, err = skipSubBlocks(data, at); err != nil {
				return 0, err
			}

		default:
			return 0, errMalformedGIF
		}
	}
}

// colourTableLen is the size in bytes of the table the packed field describes.
func colourTableLen(packed byte) int {
	return 3 << ((packed & 0x07) + 1)
}

// skipSubBlocks steps over a chain of length-prefixed blocks, returning where
// it ends. Sub-block chains are how GIF stores both extension payloads and
// compressed pixel data.
func skipSubBlocks(data []byte, at int) (int, error) {
	for {
		if at >= len(data) {
			return 0, errMalformedGIF
		}
		size := int(data[at])
		at++
		if size == 0 {
			return at, nil
		}
		at += size
		if at > len(data) {
			return 0, errMalformedGIF
		}
	}
}

// sanitizeStill decodes, applies the recorded orientation and re-encodes as
// JPEG.
//
// Re-encoding from the decoded pixels is what strips EXIF and GPS: none of the
// original metadata survives the round trip. Applying the orientation first
// matters because dropping the tag without rotating would leave every portrait
// phone photo lying on its side.
//
// AutoOrientation only reads the EXIF tag out of a JPEG. HEIC states its
// rotation in the container instead and the decoder has already applied it, so
// the two never fight over the same image.
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
