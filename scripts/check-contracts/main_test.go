package main

import (
	"slices"
	"testing"
)

func TestMissing(t *testing.T) {
	have := map[string]bool{"hello": true, "terminal.run": true}
	tests := []struct {
		want    []string
		missing []string
	}{
		{[]string{"hello", "terminal.run"}, nil},
		{[]string{"hello", "terminal.kill"}, []string{"terminal.kill"}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{}, nil},
	}
	for _, tt := range tests {
		if got := missing(have, tt.want); !slices.Equal(got, tt.missing) {
			t.Errorf("missing(%v) = %v, want %v", tt.want, got, tt.missing)
		}
	}
}
