package checks

import (
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6127PermanentFourthArchitectureReview(t *testing.T) {
	t.Parallel()
	replace := func(source, old, replacement string) string {
		t.Helper()
		if strings.Count(source, old) != 1 {
			t.Fatalf("replacement target %q count != 1", old)
		}
		return strings.Replace(source, old, replacement, 1)
	}
	beforeReturn := func(statement string) string {
		return replace(ps6127PermanentBuilder, "return c", statement+"; return c")
	}
	beforePath := func(statement string) string { return replace(ps6127PermanentMatcher, "abs:=", statement+"; abs:=") }
	cases := []struct {
		name, builder, matcher, extra string
		want                          int
	}{
		{"live control", ps6127PermanentBuilder, ps6127PermanentMatcher, "", 1},
		{"builder invoked closure clears", beforeReturn("func(){c.ignoreParts=nil}()"), ps6127PermanentMatcher, "", 0},
		{"builder bound method clears", beforeReturn("c.clear()"), ps6127PermanentMatcher, "func (c *selectorConfig) clear(){c.ignoreParts=nil}", 0},
		{"builder switch clears", beforeReturn("switch{case true:c.ignoreParts=nil}"), ps6127PermanentMatcher, "", 0},
		{"builder one-element range clears", beforeReturn("for range []int{0}{c.ignoreParts=nil}"), ps6127PermanentMatcher, "", 0},
		{"matcher invoked closure clears", ps6127PermanentBuilder, beforePath("func(){c.ignoreParts=nil}()"), "", 0},
		{"matcher bound method clears", ps6127PermanentBuilder, beforePath("c.clear()"), "func (c *selectorConfig) clear(){c.ignoreParts=nil}", 0},
		{"matcher switch returns before predicate", ps6127PermanentBuilder, beforePath("switch{case true:return false}"), "", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6127PermanentHeader + tc.builder + "\n" + tc.matcher + "\n" + tc.extra
			if got := ps6127PermanentReports(t, source, []config.RecursiveMetadataIgnoreContract{ps6127PermanentContract()}); got != tc.want {
				t.Errorf("got %d reports, want %d", got, tc.want)
			}
		})
	}
}
