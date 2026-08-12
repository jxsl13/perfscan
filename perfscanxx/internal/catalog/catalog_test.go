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

func TestUniqueTidyNames(t *testing.T) {
	seen := map[string]string{}
	for _, e := range All() {
		if prev, dup := seen[e.TidyName]; dup {
			t.Errorf("TidyName %q shared by %s and %s", e.TidyName, prev, e.ID)
		}
		seen[e.TidyName] = e.ID
	}
}

// TestCustomCheckInvariants pins the contract every query-based custom check
// must satisfy. It is the guard that would have caught PX2103 shipping without
// the isExpansionInMainFile() header gate (it fired on library-header catch
// clauses the user cannot fix), and any future Bind/.bind desync or malformed
// matcher.
func TestCustomCheckInvariants(t *testing.T) {
	custom := 0
	for _, e := range All() {
		if !e.Custom {
			// Custom-only fields must be empty on built-in entries, so a
			// stray Query/Bind can never silently ride along.
			if e.Query != "" || e.Bind != "" || e.Message != "" {
				t.Errorf("%s: built-in entry must leave Query/Bind/Message empty, got Query=%q Bind=%q Message=%q", e.ID, e.Query, e.Bind, e.Message)
			}
			continue
		}
		custom++

		// A custom check carries no clang-tidy fix-it: a matcher only reports.
		if e.HasFix {
			t.Errorf("%s: custom checks are advisory — HasFix must be false", e.ID)
		}
		// The declarative fields must all be present.
		if e.Bind == "" {
			t.Errorf("%s: custom check has empty Bind", e.ID)
		}
		if e.Message == "" {
			t.Errorf("%s: custom check has empty Message", e.ID)
		}

		q := strings.TrimSpace(e.Query)
		if !strings.HasPrefix(q, "match ") {
			t.Errorf("%s: Query must be a clang-query 'match ...' command, got %q", e.ID, q)
		}
		if !balancedParens(q) {
			t.Errorf("%s: Query has unbalanced parentheses:\n%s", e.ID, q)
		}
		// Every custom matcher must stay off standard/library headers: without
		// this guard the check fires on code the user cannot change (noise).
		if !strings.Contains(q, "isExpansionInMainFile()") {
			t.Errorf("%s: Query must contain isExpansionInMainFile() to avoid firing in library headers", e.ID)
		}
		// The diagnostic anchors on Bind, so the matcher must bind that exact
		// name — a mismatch means clang-tidy reports nothing.
		wantBind := `.bind("` + e.Bind + `")`
		if !strings.Contains(q, wantBind) {
			t.Errorf("%s: Query must bind its anchor node %s, but no %s found in:\n%s", e.ID, e.Bind, wantBind, q)
		}

		// The generated clang-tidy config must round-trip the query + bind.
		cfg := ClangTidyConfig([]Entry{e})
		name := strings.TrimPrefix(e.TidyName, "custom-")
		for _, want := range []string{"Name: " + name, "Query: |", "BindName: " + e.Bind} {
			if !strings.Contains(cfg, want) {
				t.Errorf("%s: generated config missing %q:\n%s", e.ID, want, cfg)
			}
		}
	}
	if custom == 0 {
		t.Fatal("expected at least one custom check in the catalog")
	}
}

// balancedParens reports whether every '(' has a matching ')', ignoring the
// contents of double-quoted string literals (a matcher argument like "(" must
// not throw the count off).
func balancedParens(s string) bool {
	depth := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\\' {
				i++ // skip the escaped char
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// TestEntryMetadata pins display-metadata completeness for EVERY catalog entry
// (TestUniqueIDs only covers ID/TidyName): a valid fix level and a non-empty
// Title and Category, so -list and -explain never render a blank or
// out-of-range field for any check, current or future.
func TestEntryMetadata(t *testing.T) {
	for _, e := range All() {
		if e.Title == "" {
			t.Errorf("%s: empty Title", e.ID)
		}
		if e.Category == "" {
			t.Errorf("%s: empty Category (it groups -list output)", e.ID)
		}
		if e.Level < LevelIdiomatic || e.Level > LevelAggressive {
			t.Errorf("%s: invalid Level %d (want L1..L3)", e.ID, e.Level)
		}
	}
}
