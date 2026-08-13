package catalog

import (
	"slices"
	"testing"
)

// TestSelectCompositions pins the -checks selector compositions the existing
// tests don't cover: a prefix combined with an exclusion, exclude-by-prefix, an
// explicit multi-id include set, and an exclusion with no explicit include
// (which defaults the include to "all"). These are the shapes real users type
// (e.g. "PX1*,-PX1002" to run a family minus one) and a regression in the
// include-then-exclude ordering would silently run the wrong checks.
func TestSelectCompositions(t *testing.T) {
	cases := []struct {
		selector string
		want     []string // exact expected IDs at LevelAggressive ("" entries = "no PX3xxx" style checks below)
	}{
		{"PX1*,-PX1002", []string{"PX1001", "PX1003"}},                  // family minus one
		{"PX1001,PX3003", []string{"PX1001", "PX3003"}},                 // explicit multi-id set
		{"performance-avoid-endl,PX1001", []string{"PX1001", "PX3003"}}, // tidy-name + id mix (order-independent)
	}
	for _, tc := range cases {
		got := ids(Select(tc.selector, LevelAggressive))
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("Select(%q) = %v, want %v", tc.selector, got, want)
		}
	}

	// exclude-by-prefix: "all,-PX3*" must drop every PX3xxx and keep the rest.
	got := Select("all,-PX3*", LevelAggressive)
	for _, e := range got {
		if len(e.ID) >= 3 && e.ID[:3] == "PX3" {
			t.Errorf("all,-PX3* leaked %s", e.ID)
		}
	}
	if len(got) == 0 || len(got) >= len(All()) {
		t.Errorf("all,-PX3* = %d entries, want a non-empty strict subset of %d", len(got), len(All()))
	}

	// A lone exclusion defaults the include to "all": "-PX1001" == everything
	// except PX1001.
	lone := ids(Select("-PX1001", LevelAggressive))
	if slices.Contains(lone, "PX1001") {
		t.Error("-PX1001 (lone exclusion) did not drop PX1001")
	}
	if len(lone) != len(All())-1 {
		t.Errorf("-PX1001 = %d entries, want %d (all but one)", len(lone), len(All())-1)
	}
}
