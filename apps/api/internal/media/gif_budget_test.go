package media_test

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"runtime"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/media"
)

// bombGIF builds an animation whose frames multiply out past the decode
// budget: each frame is a full canvas, and a canvas of solid colour compresses
// to almost nothing, so the file stays small while the decode does not.
func bombGIF(t *testing.T, width, height, frames int) []byte {
	t.Helper()

	palette := color.Palette{color.Black, color.White}
	animation := &gif.GIF{}
	for i := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, width, height), palette)
		// Vary one pixel so the frames are not byte-identical.
		frame.SetColorIndex(i%width, 0, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, animation); err != nil {
		t.Fatalf("encode bomb: %v", err)
	}
	return buf.Bytes()
}

// The budget has to bound the decode, not just what gets stored. Checking
// after gif.DecodeAll would reject the file only once its frames were already
// in memory, which is the whole cost of the attack.
func TestAGIFBombIsRejectedWithoutDecodingIt(t *testing.T) {
	// 4 MP a frame, 12 frames: 48 MP against a 30 MP budget.
	bomb := bombGIF(t, 2000, 2000, 12)
	t.Logf("bomb is %d KB on disk", len(bomb)/1024)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err := media.SanitizeAllowingGIF(bomb)

	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc

	if err == nil {
		t.Fatal("the bomb was accepted")
	}

	// Decoding twelve 4 MP frames costs ~48 MB. Refusing after a scan of a
	// file this size should cost orders of magnitude less; the bound is loose
	// so it fails on a real regression rather than on allocator noise.
	const budget = 8 << 20
	if allocated > budget {
		t.Fatalf("rejecting the bomb allocated %d MB, want under %d MB — it was decoded",
			allocated>>20, budget>>20)
	}
	t.Logf("rejecting it allocated %d KB", allocated/1024)
}

// The bound must not cost ordinary animations: a short GIF well inside the
// budget still goes through with its frames intact.
func TestAnOrdinaryAnimationStillPasses(t *testing.T) {
	animation := bombGIF(t, 200, 200, 24)

	out, err := media.SanitizeAllowingGIF(animation)
	if err != nil {
		t.Fatalf("a 24-frame 200x200 GIF was rejected: %v", err)
	}
	if out.Kind != media.KindGIF {
		t.Fatalf("kind = %v, want a GIF", out.Kind)
	}

	decoded, err := gif.DecodeAll(bytes.NewReader(out.Data))
	if err != nil {
		t.Fatalf("what came back is not a GIF: %v", err)
	}
	if len(decoded.Image) != 24 {
		t.Fatalf("%d frames survived, want 24", len(decoded.Image))
	}
}

// The other shape of bomb: a tiny canvas makes a pixels-only budget allow
// practically unlimited frames, while each frame still costs a fixed amount to
// decode. 250,000 frames of 4x4 fit in 7.5 MB and allocate gigabytes.
func TestAManyFrameBombOnATinyCanvasIsRejected(t *testing.T) {
	bomb := bombGIF(t, 4, 4, 250_000)
	t.Logf("bomb is %d KB on disk", len(bomb)/1024)
	if len(bomb) > media.MaxUploadBytes {
		t.Fatalf("the bomb is %d MB, over the upload limit — this shape is not reachable",
			len(bomb)>>20)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err := media.SanitizeAllowingGIF(bomb)

	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc

	if err == nil {
		t.Fatal("the bomb was accepted")
	}
	const budget = 8 << 20
	if allocated > budget {
		t.Fatalf("rejecting it allocated %d MB, want under %d MB — it was decoded",
			allocated>>20, budget>>20)
	}
	t.Logf("rejecting it allocated %d KB", allocated/1024)
}
