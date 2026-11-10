package main

import (
	"image"
	"testing"
)

func TestRenderSize(t *testing.T) {
	for _, size := range []int{16, 64, 1024} {
		img := render(size)
		if got := img.Bounds().Dx(); got != size {
			t.Fatalf("render(%d): width %d", size, got)
		}
		if got := img.Bounds().Dy(); got != size {
			t.Fatalf("render(%d): height %d", size, got)
		}
	}
}

func TestRenderCornersAreTransparent(t *testing.T) {
	img := render(64)
	for _, p := range []image.Point{{0, 0}, {63, 0}, {0, 63}, {63, 63}} {
		if _, _, _, a := img.At(p.X, p.Y).RGBA(); a != 0 {
			t.Fatalf("(%d,%d): alpha %d, want transparent", p.X, p.Y, a)
		}
	}
}

func TestPlateUsesTheIconGrid(t *testing.T) {
	img := render(256)
	// 100 design units of margin: y=80 is outside, y=110 is inside.
	if _, _, _, a := img.At(128, 20).RGBA(); a != 0 {
		t.Fatalf("y=80: alpha %d, want transparent outside the plate", a)
	}
	if _, _, _, a := img.At(128, 27).RGBA(); a != 0xffff {
		t.Fatalf("y=108: alpha %d, want an opaque plate", a)
	}
}

func TestPromptUsesTheAccent(t *testing.T) {
	img := render(design)
	for _, p := range []image.Point{
		{640, 512}, // the chevron's point
		{731, 596}, // the cursor's center
	} {
		if got := img.NRGBAAt(p.X, p.Y); got != accent {
			t.Fatalf("(%d,%d): %+v, want the accent %+v", p.X, p.Y, got, accent)
		}
	}
}

func TestPanesSplitThePlate(t *testing.T) {
	img := render(design)
	panePx := img.NRGBAAt(330, 512) // the sidebar pane
	gapPx := img.NRGBAAt(446, 512)  // the gap between the panes
	if panePx.A != 0xff || gapPx.A != 0xff {
		t.Fatalf("pane alpha %d, gap alpha %d, want both opaque", panePx.A, gapPx.A)
	}
	if panePx == gapPx {
		t.Fatalf("pane and gap are both %+v, want a visible split", panePx)
	}
}
