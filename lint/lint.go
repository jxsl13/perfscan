// Package lint defines the check metadata model of perfscan.
//
// perfscan follows the architectural shape of staticcheck: every check is a
// standard golang.org/x/tools/go/analysis.Analyzer wrapped in metadata that
// the perfscan CLI uses for listing, filtering, documentation and — the part
// that is specific to perfscan — fix-level gating.
//
// # Fix levels
//
// Performance fixes differ wildly in how much maintainability they cost.
// perfscan makes that cost explicit: every check carries a Level describing
// the character of its remedy, and `perfscan -fix` only applies fixes whose
// level is at or below the requested -fix-level.
//
//   - LevelIdiomatic (L1): the fix is idiomatic Go a reviewer waves through —
//     hoisting a regexp.MustCompile out of a loop, pre-sizing a builder or
//     slice. Deterministic, mechanical, bit-identical.
//   - LevelStructured (L2): the fix restructures code — loop interchange,
//     slab allocation, sync.Pool scratch, map→slice densification. Correct
//     but it changes the shape of the code; review and a benchmark are
//     expected.
//   - LevelAggressive (L3): the fix is a hyper-optimization — unroll-and-jam,
//     register tiling, band parallelization, branchless clamps. It buys the
//     last factor at a real maintainability price and MUST be gated by an
//     A/B benchmark and (where claimed) a bit-identity proof. Advisory by
//     default; never auto-applied without an explicit opt-in.
package lint

import "golang.org/x/tools/go/analysis"

// Level classifies the maintainability cost of a check's remedy.
type Level int

const (
	// LevelIdiomatic (L1): fix stays idiomatic; zero readability cost.
	LevelIdiomatic Level = 1
	// LevelStructured (L2): fix restructures code; moderate review cost.
	LevelStructured Level = 2
	// LevelAggressive (L3): hyper-optimized fix; high maintenance cost,
	// requires benchmark + (where claimed) bit-identity gates.
	LevelAggressive Level = 3
)

func (l Level) String() string {
	switch l {
	case LevelIdiomatic:
		return "L1"
	case LevelStructured:
		return "L2"
	case LevelAggressive:
		return "L3"
	}
	return "L?"
}

// Documentation is the long-form description of a check, rendered by
// `perfscan -explain PSxxxx` and the generated docs site.
type Documentation struct {
	// Title is a one-line summary of the anti-pattern.
	Title string
	// Text explains the pattern, the remedy and its preconditions.
	Text string
	// Before/After show a minimal example of the rewrite, if one exists.
	Before string
	After  string
	// MeasuredWin cites the benchmark evidence that justified the check.
	// perfscan checks are benchmark-verified patterns, not lint folklore.
	MeasuredWin string
}

// Check wraps an analysis.Analyzer with perfscan metadata.
type Check struct {
	// Analyzer implements the detection. Its Name must equal ID.
	Analyzer *analysis.Analyzer

	// ID is the stable PS-prefixed 4-digit identifier (e.g. "PS2005").
	// IDs are grouped by the thousands digit:
	//   PS1xxx per-element access, PS2xxx allocation, PS3xxx indirection,
	//   PS4xxx vectorization, PS5xxx arithmetic, PS6xxx verification gaps,
	//   PS7xxx offload/upload.
	// IDs are never reused: a retired check leaves a hole. They are the
	// handle an `//perfscan:ignore` directive names.
	ID string

	// Category is the short kebab-case slug of the group.
	Category string

	// Slug is the short kebab-case name of the anti-pattern.
	Slug string

	// Level is the maintainability class of the remedy (see Level docs).
	Level Level

	// AutoFix reports whether the analyzer attaches deterministic
	// SuggestedFixes that `perfscan -fix` may apply. Only checks with a
	// bit-identical mechanical rewrite set this; everything else is
	// advisory.
	AutoFix bool

	// NeedsConfig marks domain checks whose vocabulary (element accessors,
	// allocators, fan-out helpers, …) comes from a perfscan.yaml config.
	// With no vocabulary the check stays silent — and the runner says so
	// loudly, because a silent zero from a starved check reads as "no
	// instances".
	NeedsConfig bool

	// Vocab names the config fields (JSON keys) this domain check reads,
	// so the runner can warn precisely which vocabulary is missing.
	Vocab []string

	// Doc is the long-form documentation.
	Doc Documentation
}
