package runner

import (
	"slices"
	"testing"

	"github.com/jxsl13/perfscan/lint"
)

// TestSelectChecks covers the -checks selector: "all", exact IDs, trailing-*
// globs, "-"-negations, the -level cap, and the unknown-pattern error. A bug here
// would silently run the wrong set of checks.
func TestSelectChecks(t *testing.T) {
	all := []*lint.Check{
		{ID: "PS1001", Level: lint.LevelIdiomatic},
		{ID: "PS2001", Level: lint.LevelIdiomatic},
		{ID: "PS2002", Level: lint.LevelStructured},
		{ID: "PS3001", Level: lint.LevelAggressive},
		{ID: "PS3003", Level: lint.LevelIdiomatic},
	}
	ids := func(cs []*lint.Check) []string {
		out := make([]string, len(cs))
		for i, c := range cs {
			out[i] = c.ID
		}
		slices.Sort(out)
		return out
	}

	cases := []struct {
		name     string
		sel      string
		maxLevel lint.Level
		want     []string
	}{
		{"empty means all", "", lint.LevelAggressive, []string{"PS1001", "PS2001", "PS2002", "PS3001", "PS3003"}},
		{"all", "all", lint.LevelAggressive, []string{"PS1001", "PS2001", "PS2002", "PS3001", "PS3003"}},
		{"glob PS2*", "PS2*", lint.LevelAggressive, []string{"PS2001", "PS2002"}},
		{"exact id", "PS2002", lint.LevelAggressive, []string{"PS2002"}},
		{"two exact ids", "PS1001,PS3001", lint.LevelAggressive, []string{"PS1001", "PS3001"}},
		{"negation of one", "all,-PS3003", lint.LevelAggressive, []string{"PS1001", "PS2001", "PS2002", "PS3001"}},
		{"negation of a glob", "all,-PS2*", lint.LevelAggressive, []string{"PS1001", "PS3001", "PS3003"}},
		{"level cap L1 drops L2/L3", "all", lint.LevelIdiomatic, []string{"PS1001", "PS2001", "PS3003"}},
		{"level cap applies to a glob too", "PS3*", lint.LevelIdiomatic, []string{"PS3003"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := selectChecks(all, tc.sel, tc.maxLevel)
			if err != nil {
				t.Fatalf("selectChecks(%q): %v", tc.sel, err)
			}
			if g := ids(got); !slices.Equal(g, tc.want) {
				t.Errorf("selectChecks(%q, maxLevel=%d) = %v, want %v", tc.sel, tc.maxLevel, g, tc.want)
			}
		})
	}

	// An unknown pattern is an error (not a silent empty selection).
	if _, _, err := selectChecks(all, "PS9999", lint.LevelAggressive); err == nil {
		t.Error("selectChecks(PS9999): want an error for an unknown check")
	}

	// Explicitly-named exact IDs are reported in the `explicit` set.
	_, explicit, err := selectChecks(all, "PS2002,PS2*", lint.LevelAggressive)
	if err != nil {
		t.Fatal(err)
	}
	if !explicit["PS2002"] {
		t.Errorf("explicit set = %v, want it to contain the exact-named PS2002", explicit)
	}
	if explicit["PS2001"] {
		t.Errorf("explicit set = %v, PS2001 was only glob-matched, not exact-named", explicit)
	}
}
