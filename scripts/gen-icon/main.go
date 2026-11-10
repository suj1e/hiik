// Command gen-icon draws the app icon and writes it as a PNG.
//
// The mark is a split workspace: a narrow pane, a wider terminal pane and a
// prompt. The plate follows the macOS icon grid (824 of 1024, continuous
// corners, transparent outside), so Finder, the dock, Windows and Linux all
// get the same artwork from one file. At 16 px the pane fills blur into the
// plate and the prompt remains legible.
//
// It uses only the standard library, so `go run ./gen-icon` works in a fresh
// checkout. Run it from the scripts directory; the default output is the app
// module's resources/icon.png, which mygo picks up without configuration.
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

// The plate is centered with 100 units of transparent margin on each side,
// matching the macOS app icon grid.
const plateHalf = 412.0

var (
	plateTop    = color.NRGBA{R: 0x18, G: 0x21, B: 0x2b, A: 0xff}
	plateBottom = color.NRGBA{R: 0x0c, G: 0x11, B: 0x17, A: 0xff}
	pane        = color.NRGBA{R: 0x1c, G: 0x28, B: 0x36, A: 0xff}
	accent      = color.NRGBA{R: 0x5e, G: 0xea, B: 0xd4, A: 0xff}
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

// pixel is a straight-alpha color used while the layers are composited.
type pixel struct {
	r, g, b, a float64
}

// over places src on top of dst.
func over(dst, src pixel) pixel {
	a := src.a + dst.a*(1-src.a)
	if a == 0 {
		return pixel{}
	}
	return pixel{
		r: (src.r*src.a + dst.r*dst.a*(1-src.a)) / a,
		g: (src.g*src.a + dst.g*dst.a*(1-src.a)) / a,
		b: (src.b*src.a + dst.b*dst.a*(1-src.a)) / a,
		a: a,
	}
}

// solid turns a color into an opaque pixel at the given coverage.
func solid(c color.NRGBA, alpha float64) pixel {
	return pixel{
		r: float64(c.R) / 255,
		g: float64(c.G) / 255,
		b: float64(c.B) / 255,
		a: alpha,
	}
}

// render draws the icon at size×size. The plate edge is antialiased; the
// transparent margin stays empty.
func render(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := float64(size) / design
	for y := range size {
		for x := range size {
			px := (float64(x) + 0.5) / scale
			py := (float64(y) + 0.5) / scale
			c := shade(px, py, scale)
			if c.a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(clamp(c.r, 0, 1)*255 + 0.5),
				G: uint8(clamp(c.g, 0, 1)*255 + 0.5),
				B: uint8(clamp(c.b, 0, 1)*255 + 0.5),
				A: uint8(clamp(c.a, 0, 1)*255 + 0.5),
			})
		}
	}
	return img
}

// shade composites the plate, the two panes and the prompt for one point in
// design units. scale is output pixels per design unit, so distances turn
// into coverage widths.
func shade(x, y, scale float64) pixel {
	var out pixel

	if a := plateCoverage(x, y, scale); a > 0 {
		t := clamp((y-100)/(2*plateHalf), 0, 1)
		out = over(out, solid(mix(plateTop, plateBottom, t), a))
	}

	// The left pane is a sidebar; the right one is the terminal surface and
	// stays quieter so the prompt reads first.
	if a := coverage(sdRoundRect(x, y, 330, 512, 96, 212, 40) * scale); a > 0 {
		out = over(out, solid(pane, a))
	}
	if a := coverage(sdRoundRect(x, y, 646, 512, 180, 212, 40) * scale); a > 0 {
		out = over(out, solid(pane, a*0.55))
	}

	// The prompt: a chevron and the cursor after it.
	if a := coverage(sdMark(x, y) * scale); a > 0 {
		out = over(out, solid(accent, a))
	}

	return out
}

// plateCoverage returns the coverage of the superellipse plate. The exponent
// approximates the continuous corners of the macOS icon grid; the distance
// estimate is only used at the edge.
func plateCoverage(x, y, scale float64) float64 {
	const n = 5.0
	dx := math.Abs(x-512) / plateHalf
	dy := math.Abs(y-512) / plateHalf
	v := math.Pow(dx, n) + math.Pow(dy, n)
	if v <= 1 {
		return 1
	}
	return coverage((v - 1) * plateHalf * 0.25 * scale)
}

// sdMark is the signed distance to the prompt, a ">" chevron and the cursor
// after it, as capsules: rounded ends at their radius.
func sdMark(x, y float64) float64 {
	top := sdCapsule(x, y, 556, 428, 640, 512, 46)
	bottom := sdCapsule(x, y, 640, 512, 556, 596, 46)
	bar := sdCapsule(x, y, 690, 596, 772, 596, 46)
	return math.Min(math.Min(top, bottom), bar)
}

// sdCapsule is the signed distance from a point to the segment a-b.
func sdCapsule(x, y, ax, ay, bx, by, r float64) float64 {
	px, py := x-ax, y-ay
	bax, bay := bx-ax, by-ay
	h := clamp((px*bax+py*bay)/(bax*bax+bay*bay), 0, 1)
	return math.Hypot(px-h*bax, py-h*bay) - r
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

// mix interpolates two colors in sRGB.
func mix(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: uint8(clamp(float64(a.R)+(float64(b.R)-float64(a.R))*t, 0, 255) + 0.5),
		G: uint8(clamp(float64(a.G)+(float64(b.G)-float64(a.G))*t, 0, 255) + 0.5),
		B: uint8(clamp(float64(a.B)+(float64(b.B)-float64(a.B))*t, 0, 255) + 0.5),
		A: 0xff,
	}
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
