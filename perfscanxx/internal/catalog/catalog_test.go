package catalog

import (
	"strings"
	"testing"
)

func ids(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

func TestSelectAllLevels(t *testing.T) {
	all := Select("all", LevelAggressive)
	if len(all) != len(All()) {
		t.Fatalf("Select(all, L3) = %d entries, want %d", len(all), len(All()))
	}
}

func TestSelectLevelGating(t *testing.T) {
	l1 := Select("all", LevelIdiomatic)
	if len(l1) == 0 || len(l1) >= len(All()) {
		t.Fatalf("Select(all, L1) = %d entries, want a strict non-empty subset of %d", len(l1), len(All()))
	}
	for _, e := range l1 {
		if e.Level > LevelIdiomatic {
			t.Errorf("Select(all, L1) leaked %s (%s)", e.ID, e.Level)
		}
	}
	// The structured allocation checks must be gated out at L1.
	for _, id := range ids(l1) {
		if id == "PX2001" || id == "PX2002" {
			t.Errorf("L1 selection contains L2 check %s", id)
		}
	}
}

func TestSelectWildcardAndNegation(t *testing.T) {
	got := ids(Select("PX1*", LevelAggressive))
	want := []string{"PX1001", "PX1002", "PX1003"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Select(PX1*) = %v, want %v", got, want)
	}

	neg := ids(Select("all,-PX3003", LevelAggressive))
	for _, id := range neg {
		if id == "PX3003" {
			t.Error("negation -PX3003 did not exclude PX3003")
		}
	}
	if len(neg) != len(All())-1 {
		t.Errorf("Select(all,-PX3003) = %d entries, want %d", len(neg), len(All())-1)
	}
}

func TestSelectByTidyName(t *testing.T) {
	got := Select("performance-avoid-endl", LevelAggressive)
	if len(got) != 1 || got[0].ID != "PX3003" {
		t.Fatalf("Select(performance-avoid-endl) = %v, want [PX3003]", ids(got))
	}
}

func TestTidyChecksArg(t *testing.T) {
	arg := TidyChecksArg(Select("PX1001,PX3003", LevelAggressive))
	want := "-*,performance-for-range-copy,performance-avoid-endl"
	if arg != want {
		t.Errorf("TidyChecksArg = %q, want %q", arg, want)
	}
}

func TestLookups(t *testing.T) {
	if e, ok := ByTidyName("performance-for-range-copy"); !ok || e.ID != "PX1001" {
		t.Errorf("ByTidyName(performance-for-range-copy) = %+v, %v", e, ok)
	}
	if e, ok := ByID("px1002"); !ok || e.TidyName != "performance-unnecessary-value-param" {
		t.Errorf("ByID(px1002) = %+v, %v", e, ok)
	}
	if _, ok := ByID("PX9999"); ok {
		t.Error("ByID(PX9999): want !ok")
	}
}

func TestUniqueIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range All() {
		if seen[e.ID] {
			t.Errorf("duplicate catalog ID %s", e.ID)
		}
		seen[e.ID] = true
		// Built-in entries wrap a performance-* clang-tidy check; custom
		// entries are perfscanxx query-based checks named "custom-*".
		if e.Custom {
			if !strings.HasPrefix(e.TidyName, "custom-") {
				t.Errorf("%s: custom TidyName %q must start with custom-", e.ID, e.TidyName)
			}
			continue
		}
		builtin := false
		for _, p := range []string{"performance-", "modernize-", "bugprone-", "readability-", "cppcoreguidelines-"} {
			if strings.HasPrefix(e.TidyName, p) {
				builtin = true
				break
			}
		}
		if !builtin {
			t.Errorf("%s: TidyName %q is not a known clang-tidy check family", e.ID, e.TidyName)
		}
	}
}
