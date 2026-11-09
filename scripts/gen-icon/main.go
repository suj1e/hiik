// Command gen-icon draws the app icon and writes it as a PNG: a dark
// rounded square with a terminal prompt in the accent color. It uses only the
// standard library, so `go run ./gen-icon` works in a fresh checkout.
//
// Run it from the scripts directory; the default output is the app module's
// resources/icon.png, which mygo picks up without configuration.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// The icon is drawn in a 1024 unit design space and scaled to the output
// size, so the geometry below stays readable.
const design = 1024

var (
	background = color.NRGBA{R: 0x16, G: 0x1b, B: 0x21, A: 0xff}
	accent     = color.NRGBA{R: 0x5e, G: 0xea, B: 0xd4, A: 0xff}
)

func main() {
	out := flag.String("out", filepath.Join("..", "apps", "desktop", "resources", "icon.png"), "output PNG path")
	flag.Parse()

	img := render(design)
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fatal(err)
	}
	file, err := os.Create(*out)
	if err != nil {
		fatal(err)
	}
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(file, img); err != nil {
		file.Close()
		fatal(err)
	}
	if err := file.Close(); err != nil {
		fatal(err)
	}
	info, err := os.Stat(*out)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %s (%dx%d, %d bytes)\n", *out, img.Bounds().Dx(), img.Bounds().Dy(), info.Size())
}

// render draws the icon at size×size. The rounded square is inset, so the
// corners stay transparent for the shapes the platforms put behind an icon.
func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / design
	for y := range size {
		for x := range size {
			// Sample the pixel's center in design units.
			px := (float64(x) + 0.5) / scale
			py := (float64(y) + 0.5) / scale

			// Distances in pixels, so coverage is one design pixel wide.
			plate := coverage(sdRoundRect(px, py, 512, 512, 472, 472, 220) * scale)
			mark := coverage(sdMark(px, py) * scale)
			if plate == 0 && mark == 0 {
				continue
			}

			// Source-over: the accent mark on the plate. Everything is
			// premultiplied here and made straight again below.
			alpha := mark + plate*(1-mark)
			r := float64(accent.R)*mark + float64(background.R)*plate*(1-mark)
			g := float64(accent.G)*mark + float64(background.G)*plate*(1-mark)
			b := float64(accent.B)*mark + float64(background.B)*plate*(1-mark)
			if alpha == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(clamp(r/alpha, 0, 255) + 0.5),
				G: uint8(clamp(g/alpha, 0, 255) + 0.5),
				B: uint8(clamp(b/alpha, 0, 255) + 0.5),
				A: uint8(alpha*255 + 0.5),
			})
		}
	}
	return img
}

// sdMark is the signed distance to the accent mark, a ">" prompt and the
// cursor after it, as capsules: rounded ends at their radius.
func sdMark(x, y float64) float64 {
	const radius = 56
	// The chevron's arms, from its upper and lower left to the point.
	top := sdCapsule(x, y, 320, 330, 500, 512)
	bottom := sdCapsule(x, y, 320, 694, 500, 512)
	bar := sdCapsule(x, y, 588, 694, 700, 694)
	return math.Min(math.Min(top, bottom), bar) - radius
}

// sdCapsule is the signed distance from a point to the segment a-b.
func sdCapsule(x, y, ax, ay, bx, by float64) float64 {
	px, py := x-ax, y-ay
	bax, bay := bx-ax, by-ay
	// The projection of p onto a-b, clamped to the segment.
	h := clamp((px*bax+py*bay)/(bax*bax+bay*bay), 0, 1)
	return math.Hypot(px-h*bax, py-h*bay)
}

// sdRoundRect is the signed distance from a point to a rounded rectangle
// centered at (cx, cy) with half extents hw and hh and corner radius r.
func sdRoundRect(x, y, cx, cy, hw, hh, r float64) float64 {
	qx := math.Abs(x-cx) - (hw - r)
	qy := math.Abs(y-cy) - (hh - r)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	inside := math.Min(math.Max(qx, qy), 0)
	return outside + inside - r
}

// coverage turns a signed distance in pixels into how much of a pixel lies
// inside the shape, with a one pixel wide edge.
func coverage(distance float64) float64 {
	return clamp(0.5-distance, 0, 1)
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
