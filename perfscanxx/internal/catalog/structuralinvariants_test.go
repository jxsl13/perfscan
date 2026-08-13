package catalog

import (
	"regexp"
	"testing"
)

// TestCatalogStructuralInvariants pins the structural contract every catalog
// entry must satisfy, complementing TestUniqueIDs and TestCustomCheckInvariants:
// a well-formed PX#### id, a level in range, non-empty Title/Category/TidyName,
// and working ByID / ByTidyName round-trips — so -explain, -list, the selectors,
// and report attribution never hit a malformed entry.
func TestCatalogStructuralInvariants(t *testing.T) {
	idRe := regexp.MustCompile(`^PX[0-9]{4}$`)
	for _, e := range All() {
		if !idRe.MatchString(e.ID) {
			t.Errorf("%q: ID must match PX#### (four digits)", e.ID)
		}
		if e.Level < LevelIdiomatic || e.Level > LevelAggressive {
			t.Errorf("%s: level %d out of range [%d,%d]", e.ID, e.Level, LevelIdiomatic, LevelAggressive)
		}
		if e.Title == "" {
			t.Errorf("%s: empty Title (shown in -list and SARIF shortDescription)", e.ID)
		}
		if e.Category == "" {
			t.Errorf("%s: empty Category (groups -list output)", e.ID)
		}
		if e.TidyName == "" {
			t.Errorf("%s: empty TidyName", e.ID)
		}
		// ByID must round-trip (it is case-insensitive; the canonical id is upper).
		if got, ok := ByID(e.ID); !ok || got.ID != e.ID {
			t.Errorf("ByID(%s) failed to round-trip (ok=%v)", e.ID, ok)
		}
		// A built-in entry must also resolve by its TidyName — report attribution
		// maps a clang-tidy diagnostic name back to the PX id this way.
		if !e.Custom {
			if got, ok := ByTidyName(e.TidyName); !ok || got.ID != e.ID {
				t.Errorf("ByTidyName(%s) did not resolve back to %s (ok=%v)", e.TidyName, e.ID, ok)
			}
		}
	}
}
