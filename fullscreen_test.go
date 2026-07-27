package main

import "testing"

func TestParseResolution(t *testing.T) {
	cases := []struct {
		in     string
		w, h   int
		wantOK bool
	}{
		{"1920x1080", 1920, 1080, true},
		{"1280x720", 1280, 720, true},
		{"800x600", 800, 600, true},
		{"", 0, 0, false},
		{"abc", 0, 0, false},
		{"1920", 0, 0, false},
		{"1920x", 0, 0, false},
		{"x1080", 0, 0, false},
		{"0x1080", 0, 0, false},
		{"1920x0", 0, 0, false},
		{"-1920x1080", 0, 0, false},
		{"1920X1080", 0, 0, false}, // capital X is not the literal separator
	}
	for _, c := range cases {
		w, h, ok := parseResolution(c.in)
		if ok != c.wantOK {
			t.Errorf("parseResolution(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && (w != c.w || h != c.h) {
			t.Errorf("parseResolution(%q) = (%d, %d), want (%d, %d)", c.in, w, h, c.w, c.h)
		}
	}
}
