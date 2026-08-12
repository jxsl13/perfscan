package catalog

import (
	"strings"
	"testing"
)

// UnmatchedPatterns flags genuine typos (patterns matching no entry at any
// level) while accepting real checks that are merely level-gated, globs, the
// "all" wildcard, TidyName patterns, and negations.
func TestUnmatchedPatterns(t *testing.T) {
	cases := []struct {
		selector string
		want     []string
	}{
		{"PX1001,PX9999", []string{"PX9999"}}, // one typo among reals
		{"PX2001", nil},                       // real check, L2 — gated at low -level but NOT a typo
		{"PX1*", nil},                         // glob matches
		{"all", nil},                          // wildcard
		{"", nil},                             // empty selector
		{"PX1001,-PX9999", nil},               // a negation is not a typo report
		{"performance-avoid-endl,bogus-check", []string{"bogus-check"}}, // TidyName match works; typo caught
		{"NOPE,ALSO-NOPE", []string{"NOPE", "ALSO-NOPE"}},               // order preserved as given (comma order)
	}
	for _, c := range cases {
		got := UnmatchedPatterns(c.selector)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("UnmatchedPatterns(%q) = %v, want %v", c.selector, got, c.want)
		}
	}
}
