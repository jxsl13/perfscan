// Package catalog is the curated registry of clang-tidy checks that
// perfscanxx orchestrates.
//
// perfscanxx does not expose raw clang-tidy: it ships a CURATED catalog in
// which every entry carries a stable PX id and a fix level, mirroring
// perfscan's graded model:
//
//	L1 idiomatic   mechanical rewrites any reviewer waves through
//	L2 structured  restructures code; review + benchmark expected
//	L3 aggressive  hyper-optimizations; opt-in, benchmark-gated
//
// One -level knob gates both reporting and fixing: `-level 1 -fix` runs
// clang-tidy with only the L1 checks enabled, so clang-tidy's -fix applies
// exactly the reported checks' fix-its.
package catalog

import "strings"

// Level classifies the maintainability cost of a check's remedy.
type Level int

const (
	// LevelIdiomatic (L1): fix stays idiomatic; zero readability cost.
	LevelIdiomatic Level = 1
	// LevelStructured (L2): fix restructures code; moderate review cost.
	LevelStructured Level = 2
	// LevelAggressive (L3): hyper-optimized fix; high maintenance cost.
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

// Entry maps one clang-tidy check into the perfscanxx model.
type Entry struct {
	// ID is the stable PX-prefixed identifier (e.g. "PX1001").
	// PX1xxx copies, PX2xxx allocation, PX3xxx moves/strings.
	ID string
	// TidyName is the exact clang-tidy check name.
	TidyName string
	// Level gates reporting and fixing (see package doc).
	Level Level
	// Category groups related checks in -list output.
	Category string
	// Title is a one-line summary of the anti-pattern.
	Title string
	// HasFix reports whether clang-tidy emits fix-its for this check.
	HasFix bool
}

// entries is the seed catalog. Every TidyName is a real clang-tidy
// performance-* check (see clang.llvm.org/extra/clang-tidy/checks/list.html).
var entries = []Entry{
	{
		ID: "PX1001", TidyName: "performance-for-range-copy",
		Level: LevelIdiomatic, Category: "copies",
		Title:  "range-for loop variable copies each element; take a const reference",
		HasFix: true,
	},
	{
		ID: "PX1002", TidyName: "performance-unnecessary-value-param",
		Level: LevelIdiomatic, Category: "copies",
		Title:  "expensive parameter passed by value; pass by const reference",
		HasFix: true,
	},
	{
		ID: "PX1003", TidyName: "performance-unnecessary-copy-initialization",
		Level: LevelIdiomatic, Category: "copies",
		Title:  "local copy of a never-modified object; bind a const reference",
		HasFix: true,
	},
	{
		ID: "PX2001", TidyName: "performance-inefficient-vector-operation",
		Level: LevelStructured, Category: "allocation",
		Title:  "push_back in a counted loop without reserve(); pre-size the vector",
		HasFix: false,
	},
	{
		ID: "PX2002", TidyName: "performance-inefficient-string-concatenation",
		Level: LevelStructured, Category: "allocation",
		Title:  "repeated operator+ string concatenation; use += or append",
		HasFix: false,
	},
	{
		ID: "PX3001", TidyName: "performance-move-const-arg",
		Level: LevelIdiomatic, Category: "moves",
		Title:  "std::move of a const or trivially-copyable value has no effect",
		HasFix: true,
	},
	{
		ID: "PX3002", TidyName: "performance-faster-string-find",
		Level: LevelIdiomatic, Category: "strings",
		Title:  "find() of a single-character literal; use the char overload",
		HasFix: true,
	},
	{
		ID: "PX3003", TidyName: "performance-avoid-endl",
		Level: LevelIdiomatic, Category: "io",
		Title:  "std::endl flushes the stream every time; use '\\n'",
		HasFix: true,
	},
}

// All returns the full catalog in stable ID order.
func All() []Entry {
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

// ByTidyName resolves a clang-tidy check name to its catalog entry.
func ByTidyName(name string) (Entry, bool) {
	for _, e := range entries {
		if e.TidyName == name {
			return e, true
		}
	}
	return Entry{}, false
}

// ByID resolves a PX id (case-insensitive) to its catalog entry.
func ByID(id string) (Entry, bool) {
	id = strings.ToUpper(id)
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Select filters the catalog with a perfscan-style selector and a level cap.
//
// selector is a comma-separated list of patterns: "all", an ID ("PX1001"),
// a tidy name ("performance-avoid-endl"), a trailing-* glob on either
// ("PX1*", "performance-*"), and "-"-prefixed negations ("all,-PX3003").
// Entries with Level > maxLevel are always excluded: ONE knob gates both
// reporting and fixing.
func Select(selector string, maxLevel Level) []Entry {
	include := make([]string, 0, 4)
	exclude := make([]string, 0, 4)
	for _, pat := range strings.Split(selector, ",") {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if neg, ok := strings.CutPrefix(pat, "-"); ok {
			exclude = append(exclude, neg)
		} else {
			include = append(include, pat)
		}
	}
	if len(include) == 0 {
		include = append(include, "all")
	}

	var out []Entry
	for _, e := range entries {
		if e.Level > maxLevel {
			continue
		}
		if !matchAny(e, include) || matchAny(e, exclude) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func matchAny(e Entry, pats []string) bool {
	for _, p := range pats {
		if match(e, p) {
			return true
		}
	}
	return false
}

func match(e Entry, pat string) bool {
	if pat == "all" || pat == "*" {
		return true
	}
	if prefix, ok := strings.CutSuffix(pat, "*"); ok {
		return strings.HasPrefix(strings.ToUpper(e.ID), strings.ToUpper(prefix)) ||
			strings.HasPrefix(e.TidyName, prefix)
	}
	return strings.EqualFold(e.ID, pat) || e.TidyName == pat
}

// TidyChecksArg renders entries as a clang-tidy -checks= value that disables
// everything else: "-*,performance-for-range-copy,...".
func TidyChecksArg(sel []Entry) string {
	parts := make([]string, 0, len(sel)+1)
	parts = append(parts, "-*")
	for _, e := range sel {
		parts = append(parts, e.TidyName)
	}
	return strings.Join(parts, ",")
}
