package main

import (
	"image"
	"image/color"
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

func TestRenderPlateAndMark(t *testing.T) {
	img := render(128)
	// Top edge, horizontally centered: plate, not the mark.
	if _, _, _, a := img.At(64, 8).RGBA(); a != 0xffff {
		t.Fatalf("top center: alpha %d, want opaque plate", a)
	}
	// The center is the accent mark, whose point is there.
	got := color.NRGBAModel.Convert(img.At(64, 64)).(color.NRGBA)
	if got.A != 0xff {
		t.Fatalf("center: alpha %d, want opaque", got.A)
	}
	for _, c := range []struct {
		name      string
		got, want uint8
	}{
		{"R", got.R, accent.R},
		{"G", got.G, accent.G},
		{"B", got.B, accent.B},
	} {
		if c.got != c.want {
			t.Fatalf("center: %s %d, want %d", c.name, c.got, c.want)
		}
	}
}
