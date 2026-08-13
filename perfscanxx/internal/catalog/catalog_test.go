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
	// An unknown TidyName resolves to no entry (the -explain/-list/selector
	// paths rely on this miss returning a zero Entry, not a stale match).
	if e, ok := ByTidyName("performance-no-such-check"); ok || e.ID != "" {
		t.Errorf("ByTidyName(unknown) = %+v, %v; want zero Entry, false", e, ok)
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

// TestAggressiveOnlyPerfChecks pins PX3021/PX3022: they are genuine clang-tidy
// perf diagnostics with no fix-it, and must be gated to L3 so they never spam
// at the default level. Regression guard for the no-spam / level-gating policy.
func TestAggressiveOnlyPerfChecks(t *testing.T) {
	for _, id := range []string{"PX3021", "PX3022"} {
		e, ok := ByID(id)
		if !ok {
			t.Fatalf("%s missing from catalog", id)
		}
		if e.Level != LevelAggressive {
			t.Errorf("%s: Level = %v, want LevelAggressive (L3 — niche, must not surface by default)", id, e.Level)
		}
		if e.HasFix {
			t.Errorf("%s: HasFix = true, but clang-tidy emits no fix-it for it (advisory-only)", id)
		}
	}
	// Absent below L3, present at L3.
	l2 := ids(Select("all", LevelStructured))
	l3 := ids(Select("all", LevelAggressive))
	for _, id := range []string{"PX3021", "PX3022"} {
		for _, got := range l2 {
			if got == id {
				t.Errorf("%s leaked into an L2 selection; it is L3-only", id)
			}
		}
		found := false
		for _, got := range l3 {
			if got == id {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not present in an L3 selection", id)
		}
	}
}

// TestLevelString pins the L1/L2/L3 labels the -list output and messages use,
// plus the defensive "L?" for an out-of-range Level (so a bad catalog Level
// renders visibly rather than as an empty string).
func TestLevelString(t *testing.T) {
	cases := map[Level]string{
		LevelIdiomatic:  "L1",
		LevelStructured: "L2",
		LevelAggressive: "L3",
		Level(0):        "L?",
		Level(99):       "L?",
	}
	for lvl, want := range cases {
		if got := lvl.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", int(lvl), got, want)
		}
	}
}

// TestCaveatsAreWellFormed pins the Caveat invariants: a caveat is only
// meaningful on an auto-fixable check (it warns about clang-tidy's fix-it), so
// no advisory/custom entry may carry one; and the checks whose upstream fix-it
// is known to be unsafe-or-a-tradeoff-if-applied-blindly must keep their caveat
// so `-explain` warns reviewers: PX3015 (member-init hoists reads past a
// constructor-body lock) and PX3004 (noexcept may be omitted on purpose when a
// member op can throw), both found during corpus validation on spdlog/fmt; and
// PX3007 (pass-by-value pessimizes lvalue callers and changes copy/move counts).
func TestCaveatsAreWellFormed(t *testing.T) {
	for _, e := range All() {
		if e.Caveat != "" && !e.HasFix {
			t.Errorf("%s has a Caveat but HasFix=false; caveats only apply to fixable checks", e.ID)
		}
	}
	for _, id := range []string{"PX3015", "PX3004", "PX3007"} {
		e, ok := ByID(id)
		if !ok {
			t.Fatalf("%s missing from catalog", id)
		}
		if e.Caveat == "" {
			t.Errorf("%s must carry a Caveat (upstream fix-it is unsafe to apply blindly)", id)
		}
	}
}

// TestAuditedExclusionsStayExcluded records the result of a clang-tidy 22
// perf/modernize/readability audit (checks NOT in the catalog) and pins the
// DELIBERATE exclusions so a future audit does not re-add them without the
// rationale. Empirically (clang-tidy --fix on a crafted fixture):
//
//   - performance-move-constructor-init, performance-noexcept-destructor,
//     performance-type-promotion-in-math-fn: these are genuine perf checks but
//     apply NO fix-it on this toolchain (macOS/libc++) — HasFix would be false,
//     so they offer no auto-fix value over the advisory set already documented.
//
// CORRECTION (later audit): performance-trivially-destructible was previously
// listed here as "no fix-it", but that was a false negative — the earlier probe
// used an in-class empty/defaulted destructor, which the check does NOT target.
// The check fires on an OUT-OF-LINE defaulted destructor (~S(); then
// S::~S() = default;), and for that exact shape `clang-tidy --fix` DOES apply a
// working fix-it: it defaults the destructor on the first declaration and drops
// the out-of-line definition, restoring trivial destructibility (a strict perf
// win — enables trivial relocation, elides per-element destructor calls). It is
// now catalog entry PX3026 (HasFix:true, verified end-to-end by
// TestHasFixChecksActuallyApply). LESSON: probe a check with the EXACT AST shape
// it matches before concluding "no fix-it".
//   - modernize-min-max-use-initializer-list: its fix-it DOES apply
//     (std::max(a,std::max(b,c)) -> std::max({a,b,c})) and is bit-identical for
//     integers, BUT (1) it is a readability modernization with NO perf angle
//     (identical comparison count), and (2) for FLOAT args it can diverge on
//     NaN ordering (the same float-NaN hazard perfscan's min/max checks
//     exclude). A pure-readability check with a NaN footgun does not meet the
//     perf-catalog bar. If ever added, it MUST carry a Caveat.
//
// Removing an entry here is a deliberate decision, not a mechanical bump: read
// the rationale first.
func TestAuditedExclusionsStayExcluded(t *testing.T) {
	excluded := []string{
		"modernize-min-max-use-initializer-list",
		"performance-move-constructor-init",
		"performance-noexcept-destructor",
		"performance-type-promotion-in-math-fn",
	}
	for _, name := range excluded {
		if _, ok := ByTidyName(name); ok {
			t.Errorf("%s is in the catalog but was deliberately excluded (see this test's rationale); if adding it intentionally, update the audit note", name)
		}
	}
}
