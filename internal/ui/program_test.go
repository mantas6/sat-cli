package ui

import (
	"io"
	"testing"
)

func TestInitialSizeKeepsKnownDimensionsAndFallsBack(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   Size
		want Size
	}{
		{name: "known", in: Size{Width: 100, Height: 30}, want: Size{Width: 100, Height: 30}},
		{name: "unknown", in: Size{}, want: Size{Width: 80, Height: 24}},
		{name: "width only", in: Size{Width: 50}, want: Size{Width: 50, Height: 24}},
		{name: "negative height", in: Size{Width: 50, Height: -1}, want: Size{Width: 50, Height: 24}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// io.Discard is not a terminal, so only the fallback applies.
			if got := initialSize(test.in, io.Discard); got != test.want {
				t.Fatalf("initialSize(%v) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}
