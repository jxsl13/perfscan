package checks

import (
	"regexp"
	"testing"
)

// TestEquivPS5046 proves the rewrite is byte-identical: every single-argument
// Find* method returns non-nil exactly when the pattern matches, so Find*(x) !=
// nil == Match/MatchString(x) (and == nil is the negation) for every subject,
// the empty-match case included.
func TestEquivPS5046(t *testing.T) {
	patterns := []string{"", "a", "ab+c", "^\\d+$", "(x)(y)?", "a*", "世界", ".*"}
	inputs := []string{"", "a", "abbbc", "123", "世界", "\xff no", "x", "xy"}
	for _, p := range patterns {
		re := regexp.MustCompile(p)
		for _, s := range inputs {
			b := []byte(s)
			ms, m := re.MatchString(s), re.Match(b)
			if (re.FindStringIndex(s) != nil) != ms ||
				(re.FindStringSubmatch(s) != nil) != ms ||
				(re.FindStringSubmatchIndex(s) != nil) != ms {
				t.Fatalf("string Find*/MatchString diverge p=%q s=%q", p, s)
			}
			if (re.Find(b) != nil) != m ||
				(re.FindIndex(b) != nil) != m ||
				(re.FindSubmatch(b) != nil) != m ||
				(re.FindSubmatchIndex(b) != nil) != m {
				t.Fatalf("bytes Find*/Match diverge p=%q b=%q", p, b)
			}
		}
	}
}
