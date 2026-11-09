package main

import (
	"slices"
	"testing"
)

func TestLinkTargets(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"see [the docs](../docs/readme.md).", []string{"../docs/readme.md"}},
		{"[a](x.md) and [b](y.md#frag)", []string{"x.md", "y.md#frag"}},
		{"[title](path.md \"A title\")", []string{"path.md"}},
		{"[in code](`nope.md`)", nil},
		{"`[code](nope.md)` but [real](yes.md)", []string{"yes.md"}},
		{"![image](icon.png)", []string{"icon.png"}},
		{"no links here", nil},
	}
	for _, tt := range tests {
		if got := linkTargets(tt.line); !slices.Equal(got, tt.want) {
			t.Errorf("linkTargets(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestFencedLines(t *testing.T) {
	text := "# Title\n\n```sh\n[not](a.md)\n```\n[yes](b.md)\n"
	want := []bool{false, false, false, true, true, false, false}
	if got := fencedLines(text); !slices.Equal(got, want) {
		t.Fatalf("fencedLines = %v, want %v", got, want)
	}
}

func TestFencedLinesUnclosed(t *testing.T) {
	// An unclosed fence keeps the rest of the document inside it, rather
	// than reporting links in it.
	text := "text\n```\n[inside](a.md)\n"
	want := []bool{false, false, true, true}
	if got := fencedLines(text); !slices.Equal(got, want) {
		t.Fatalf("fencedLines = %v, want %v", got, want)
	}
}
