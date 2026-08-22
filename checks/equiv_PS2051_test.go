package checks

import (
	"regexp"
	"testing"
)

// TestEquivPS2051 proves the rewrite is byte-identical: for a compiled *Regexp,
// re.Match([]byte(s)) == re.MatchString(s) and re.MatchString(string(b)) ==
// re.Match(b) for every subject — Match and MatchString run the same automaton
// over the same bytes, including invalid UTF-8.
func TestEquivPS2051(t *testing.T) {
	patterns := []string{"", "a", "ab+c", "^\\d+$", "[[:space:]]", ".", "(?i)go+d",
		"\\p{L}+", "a.c", "^$", "x*", "世界"}
	inputs := []string{"", "a", "abc", "abbbc", "  ", "123", "世界", "GOOOD",
		"\xff\xfe invalid", "\x80lone", "mixed 世\xffz", "line1\nline2"}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("compile %q: %v", p, err)
		}
		for _, s := range inputs {
			if re.Match([]byte(s)) != re.MatchString(s) {
				t.Fatalf("Match/MatchString diverge p=%q s=%q", p, s)
			}
			b := []byte(s)
			if re.MatchString(string(b)) != re.Match(b) {
				t.Fatalf("MatchString/Match diverge p=%q b=%q", p, b)
			}
		}
	}
}
