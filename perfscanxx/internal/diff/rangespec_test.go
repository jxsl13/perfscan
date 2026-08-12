package diff

import "testing"

// rangeSpec formats a unified-diff hunk range. git/patch require "start" (not
// "start,1") for a single-line range, "start,0" for an empty side, and
// "start,count" otherwise — a wrong spec makes the emitted patch unappliable.
func TestRangeSpec(t *testing.T) {
	cases := []struct {
		start, count int
		want         string
	}{
		{5, 1, "5"},   // single line: no trailing ",1"
		{0, 0, "0,0"}, // empty side (a pure insertion into an empty file)
		{1, 0, "1,0"}, // empty side at line 1
		{2, 3, "2,3"}, // multi-line range
		{10, 7, "10,7"},
	}
	for _, c := range cases {
		if got := rangeSpec(c.start, c.count); got != c.want {
			t.Errorf("rangeSpec(%d,%d) = %q, want %q", c.start, c.count, got, c.want)
		}
	}
}
