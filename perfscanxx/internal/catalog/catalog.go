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

	// Custom marks a perfscanxx-defined query-based custom check (run via
	// clang-tidy --experimental-custom-checks). Its TidyName is the
	// "custom-<name>" clang-tidy emits. For these, Query/Bind/Message below
	// define the check declaratively — NO compiled C++.
	Custom bool
	// Query is the clang-query matcher (a "match ..." command binding Bind).
	Query string
	// Bind is the matcher's bound node name the diagnostic anchors on.
	Bind string
	// Message is the diagnostic text for a custom check.
	Message string
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
		HasFix: true, // clang-tidy inserts v.reserve(n) before the loop
	},
	{
		ID: "PX2003", TidyName: "modernize-use-emplace",
		Level: LevelIdiomatic, Category: "allocation",
		Title:  "push_back(T(args)) constructs a temporary then moves; emplace_back(args) builds in place",
		HasFix: true,
	},
	{
		ID: "PX2004", TidyName: "modernize-make-shared",
		Level: LevelIdiomatic, Category: "allocation",
		Title:  "shared_ptr(new T) does two allocations; make_shared does one",
		HasFix: true,
	},
	{
		ID: "PX2005", TidyName: "modernize-make-unique",
		Level: LevelIdiomatic, Category: "allocation",
		Title:  "unique_ptr(new T) -> make_unique<T>(...) (clearer, exception-safe)",
		HasFix: true,
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
	{
		ID: "PX3004", TidyName: "performance-noexcept-move-constructor",
		Level: LevelStructured, Category: "moves",
		Title:  "a move constructor/assignment not marked noexcept forces containers to copy; add noexcept",
		HasFix: true,
	},
	{
		ID: "PX3005", TidyName: "performance-inefficient-algorithm",
		Level: LevelStructured, Category: "algorithms",
		Title:  "std::find/count over an associative container; use the container's O(log n) member",
		HasFix: true,
	},
	{
		ID: "PX3006", TidyName: "performance-noexcept-swap",
		Level: LevelStructured, Category: "moves",
		Title:  "a swap() not marked noexcept blocks the noexcept-swap optimization; add noexcept",
		HasFix: true,
	},
	{
		ID: "PX3007", TidyName: "modernize-pass-by-value",
		Level: LevelStructured, Category: "moves",
		Title:  "sink parameter taken by const& then copied; take by value and std::move (one copy or move, not always a copy)",
		HasFix: true,
	},
	{
		ID: "PX3008", TidyName: "readability-container-size-empty",
		Level: LevelIdiomatic, Category: "containers",
		Title:  "size() == 0 to test emptiness; empty() is guaranteed O(1) (size() can be O(n), e.g. std::list)",
		HasFix: true,
	},
	{
		ID: "PX3009", TidyName: "readability-redundant-string-cstr",
		Level: LevelIdiomatic, Category: "strings",
		Title:  "s.c_str() where a std::string is expected reconstructs a string (strlen + copy); pass s directly",
		HasFix: true,
	},
	{
		ID: "PX3010", TidyName: "readability-container-contains",
		Level: LevelIdiomatic, Category: "containers",
		Title:  "count()/find() != end() membership test; .contains() is clearer and never over-counts (C++20)",
		HasFix: true,
	},
	{
		ID: "PX3011", TidyName: "readability-const-return-type",
		Level: LevelStructured, Category: "moves",
		Title:  "top-level const on a by-value return type pessimizes move at call sites; drop the const",
		HasFix: true,
	},
	{
		ID: "PX3012", TidyName: "modernize-use-transparent-functors",
		Level: LevelStructured, Category: "containers",
		Title:  "std::less<T> etc. as a container comparator; the transparent std::less<> enables heterogeneous lookup without temporaries",
		HasFix: true,
	},
	{
		ID: "PX3013", TidyName: "modernize-use-equals-default",
		Level: LevelStructured, Category: "moves",
		Title:  "empty-body special member ({}) instead of = default; the user-provided body makes the type non-trivial and blocks memcpy/trivial-copy optimizations",
		HasFix: true,
	},
	{
		ID: "PX3014", TidyName: "readability-string-compare",
		Level: LevelIdiomatic, Category: "strings",
		Title:  "s.compare(t) == 0 for (in)equality; s == t / s != t is clearer and length-checks first",
		HasFix: true,
	},
	{
		ID: "PX3015", TidyName: "cppcoreguidelines-prefer-member-initializer",
		Level: LevelStructured, Category: "copies",
		Title:  "member assigned in the constructor body default-constructs then assigns; a member initializer constructs it once directly",
		HasFix: true,
	},
	{
		ID: "PX3016", TidyName: "modernize-avoid-bind",
		Level: LevelStructured, Category: "callables",
		Title:  "std::bind carries type-erasure overhead and inhibits inlining; an equivalent lambda is cheaper and inlinable",
		HasFix: true,
	},
	{
		ID: "PX3018", TidyName: "readability-redundant-string-init",
		Level: LevelIdiomatic, Category: "strings",
		Title:  `std::string s = "" constructs from a C string; the default constructor is empty and cheaper`,
		HasFix: true,
	},
	{
		ID: "PX3019", TidyName: "modernize-use-starts-ends-with",
		Level: LevelIdiomatic, Category: "strings",
		Title:  "find(prefix)==0 / rfind(suffix)==size-n scans on mismatch; starts_with/ends_with short-circuits (C++20)",
		HasFix: true,
	},
	{
		ID: "PX3023", TidyName: "modernize-shrink-to-fit",
		Level: LevelIdiomatic, Category: "allocation",
		Title:  "the vector<T>(v).swap(v) copy-and-swap idiom to shed excess capacity always allocates a full copy; v.shrink_to_fit() requests it directly and may skip the copy when already fit (C++11)",
		HasFix: true,
	},
	{
		// Advisory (clang-tidy emits no fix-it — Replacements is empty): the loop
		// variable's type differs from the element type the iterator yields, so
		// every iteration performs an implicit conversion that materializes a
		// temporary bound to the reference. The remedy is a judgment call
		// clang-tidy won't make mechanically — match the element type, use
		// `const auto&`, or drop the reference to make the per-element copy
		// explicit — so it stays advisory. A cheap conversion (e.g. int->float on
		// a hot reduction) can be intentional, so it is L2 (review), never a
		// default-level auto-fix.
		ID: "PX3024", TidyName: "performance-implicit-conversion-in-loop",
		Level: LevelStructured, Category: "copies",
		Title:  "a range-for loop variable whose type differs from the element type converts (materializing a temporary) every iteration; match the type or use const auto&",
		HasFix: false,
	},
	{
		// Advisory (clang-tidy emits no fix-it): the corrected std::move can't be
		// inserted mechanically without knowing the parameter is dead afterwards.
		// KNOWN false positives: clang-tidy only recognizes std::move(param) /
		// std::forward(param). A parameter consumed some OTHER way — member-wise
		// (std::move(other.member_)) or via std::make_move_iterator(other.begin())
		// — is still flagged though it IS moved-from. This mostly hits container /
		// allocator-extended move constructors (e.g. abseil), so it is near-silent
		// on ordinary application code; see examples/validation.md.
		ID: "PX3020", TidyName: "cppcoreguidelines-rvalue-reference-param-not-moved",
		Level: LevelStructured, Category: "moves",
		Title:  "an rvalue-reference parameter never std::move'd is a missed move — it copies where it could have moved",
		HasFix: false,
	},
	{
		// Advisory (clang-tidy emits no fix-it): an integer<->pointer cast
		// (reinterpret_cast<T*>(intval) or the reverse) opaques the value to the
		// optimizer — it defeats alias analysis and blocks optimizations across
		// the cast. There is no mechanical rewrite (the fix is to redesign so the
		// pointer is never round-tripped through an integer), so it stays advisory.
		// Gated to L3: it is a niche systems/embedded pattern, near-silent on
		// ordinary application code, so it must never surface at the default level.
		ID: "PX3021", TidyName: "performance-no-int-to-ptr",
		Level: LevelAggressive, Category: "codegen",
		Title:  "an integer<->pointer cast pessimizes optimization — it defeats the optimizer's alias analysis",
		HasFix: false,
	},
	{
		// Advisory (clang-tidy emits no fix-it, only a suggested base type): an
		// enum whose fixed underlying type is wider than its value set needs wastes
		// storage in every object and array that holds it. The suggested narrower
		// type is context-dependent (ABI, arithmetic, forward-declares elsewhere),
		// so it is not a safe mechanical rewrite and stays advisory. Gated to L3 to
		// avoid noise on deliberately-wide enums.
		ID: "PX3022", TidyName: "performance-enum-size",
		Level: LevelAggressive, Category: "layout",
		Title:  "an enum's underlying type is wider than its value set needs; a narrower base type shrinks every instance",
		HasFix: false,
	},
	// Query-based custom check (ZERO compiled C++) — the C++ analog of the
	// Go linter's PS2101. Run via clang-tidy --experimental-custom-checks.
	{
		ID: "PX2101", TidyName: "custom-reserve-before-loop",
		Level: LevelStructured, Category: "allocation",
		Title:  "vector grown via push_back/emplace_back in a loop with no prior reserve()",
		HasFix: false,
		Custom: true,
		Bind:   "grow",
		// Any loop kind — forStmt alone missed range-for (the most common C++
		// loop) and while/do loops. isExpansionInMainFile keeps it off headers.
		Query: `match cxxMemberCallExpr(isExpansionInMainFile(), ` +
			`callee(cxxMethodDecl(hasAnyName("push_back", "emplace_back"))), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("grow")`,
		Message: "vector grown via push_back/emplace_back inside a loop; reserve() before the loop to avoid repeated reallocation (perfscanxx PS2101 analog, query-based)",
	},
	{
		ID: "PX2102", TidyName: "custom-pessimizing-move",
		Level: LevelStructured, Category: "moves",
		Title:  "return std::move(local) blocks copy/move elision (NRVO); return the local directly",
		HasFix: false,
		Custom: true,
		Bind:   "mv",
		// isExpansionInMainFile keeps it off headers (inline/template returns).
		Query: `match returnStmt(isExpansionInMainFile(), hasReturnValue(ignoringParenImpCasts(` +
			`cxxConstructExpr(hasArgument(0, ignoringParenImpCasts(` +
			`callExpr(callee(functionDecl(hasName("::std::move"))), ` +
			`hasArgument(0, declRefExpr(to(varDecl(hasLocalStorage()))))).bind("mv")))))))`,
		Message: "std::move of a local in a return statement pessimizes NRVO — the compiler can elide the copy/move entirely if you return the local directly (query-based, no auto-fix)",
	},
	{
		ID: "PX2103", TidyName: "custom-catch-by-value",
		Level: LevelIdiomatic, Category: "copies",
		Title:  "exception caught by value copies (and can slice) it; catch by const reference",
		HasFix: false,
		Custom: true,
		Bind:   "cv",
		// isExpansionInMainFile keeps it off library/system headers (e.g. an
		// inline function's catch clause in a third-party header the user can't
		// change) — consistent with every other custom check.
		Query:   `match cxxCatchStmt(isExpansionInMainFile(), has(varDecl(hasType(hasCanonicalType(recordType()))).bind("cv")))`,
		Message: "exception caught by value copies the exception object (and can slice a derived type to its base); catch by const reference (query-based, no auto-fix)",
	},
	{
		ID: "PX2104", TidyName: "custom-regex-in-loop",
		Level: LevelStructured, Category: "allocation",
		Title:  "std::regex constructed inside a loop recompiles the pattern every iteration; hoist it out",
		HasFix: false,
		Custom: true,
		Bind:   "rx",
		// A std::regex/std::wregex variable declared inside any loop kind. The
		// canonical-type match sees through the std::regex typedef to
		// basic_regex<>; isExpansionInMainFile keeps it off the <regex> header's
		// own internal constructions. The C++ analog of perfscan's PS2005.
		Query: `match varDecl(isExpansionInMainFile(), ` +
			`hasType(hasCanonicalType(recordType(hasDeclaration(cxxRecordDecl(matchesName("basic_regex")))))), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("rx")`,
		Message: "std::regex/std::wregex constructed inside a loop recompiles the pattern (an expensive parse + allocation) every iteration; declare it once outside the loop (query-based, no auto-fix)",
	},
	{
		ID: "PX2105", TidyName: "custom-dynamic-cast-in-loop",
		Level: LevelStructured, Category: "algorithms",
		Title:  "dynamic_cast inside a loop pays an RTTI type-check every iteration",
		HasFix: false,
		Custom: true,
		Bind:   "dc",
		// dynamic_cast (a runtime RTTI lookup) evaluated inside any loop kind.
		// isExpansionInMainFile keeps it off standard/library headers. No safe
		// mechanical fix — the remedy (hoist the cast, use virtual dispatch, or a
		// cheaper type discriminator) is a design change.
		Query: `match cxxDynamicCastExpr(isExpansionInMainFile(), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("dc")`,
		Message: "dynamic_cast inside a loop pays an RTTI type-check on every iteration; hoist the cast out, replace it with virtual dispatch, or use a cheaper type discriminator (query-based, no auto-fix)",
	},
	{
		ID: "PX2106", TidyName: "custom-stringstream-in-loop",
		Level: LevelStructured, Category: "allocation",
		Title:  "std::stringstream constructed inside a loop reallocates its buffer every iteration",
		HasFix: false,
		Custom: true,
		Bind:   "ss",
		// A std::(o|i)stringstream declared inside any loop kind: each
		// construction heap-allocates a fresh buffer and imbues the locale, all
		// re-done every iteration. isExpansionInMainFile keeps it off library
		// headers. No safe mechanical fix — the remedy (hoist the stream out and
		// reset it with .str("") each iteration) is a design change.
		Query: `match varDecl(isExpansionInMainFile(), ` +
			`hasType(hasCanonicalType(recordType(hasDeclaration(cxxRecordDecl(matchesName("::std::basic_(o|i)?stringstream")))))), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("ss")`,
		Message: "std::stringstream constructed inside a loop heap-allocates a new buffer (and re-imbues the locale) every iteration; hoist the stream out of the loop and reset it with .str(\"\") each pass (query-based, no auto-fix)",
	},
}

// Deliberately NOT in the catalog (do not re-add on a future audit):
//
//   - modernize-use-default-member-init — NOT a performance check (identical
//     codegen; the real perf case, moving a constructor body assignment into
//     the member-initializer list, is PX3015 prefer-member-initializer). Worse,
//     its fix-it is UNSAFE on real code: for a member with a trailing attribute
//     macro it inserts the brace-init before the attribute, e.g.
//     `T* p_ GUARDED_BY(m_);` becomes `T* p_{nullptr} GUARDED_BY(m_);`, which
//     fails to compile ("expected ';'") wherever GUARDED_BY/ABSL_GUARDED_BY
//     expands to a real thread-safety attribute. Empirically broke leveldb's
//     build under `-fix` (db_impl.h). See examples/validation.md.
//   - performance-type-promotion-in-math-fn, performance-move-constructor-init,
//     performance-inefficient-string-concatenation — clang-tidy emits no fix-it.

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

// DocURL returns the upstream clang-tidy documentation URL for a catalog entry,
// namespaced by check family (checks/<family>/<name>.html). ok is false for
// query-based custom checks, which are perfscanxx-defined and have no upstream
// page. Shared by `-explain` and the SARIF helpUri so the two never drift.
func DocURL(e Entry) (url string, ok bool) {
	if e.Custom {
		return "", false
	}
	family, name, found := strings.Cut(e.TidyName, "-")
	if !found {
		return "", false
	}
	return "https://clang.llvm.org/extra/clang-tidy/checks/" + family + "/" + name + ".html", true
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
	for pat := range strings.SplitSeq(selector, ",") {
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

// UnmatchedPatterns returns the non-negation include patterns in selector that
// match NO catalog entry at any level — genuine typos (e.g. "PX9999"), as
// distinct from a real check merely gated out by -level (which Select drops but
// match() still recognizes). Negations and the empty selector yield nothing.
func UnmatchedPatterns(selector string) []string {
	var unmatched []string
	for pat := range strings.SplitSeq(selector, ",") {
		pat = strings.TrimSpace(pat)
		if pat == "" || strings.HasPrefix(pat, "-") {
			continue
		}
		matched := false
		for _, e := range entries {
			if match(e, pat) {
				matched = true
				break
			}
		}
		if !matched {
			unmatched = append(unmatched, pat)
		}
	}
	return unmatched
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

// AnyCustom reports whether the selection contains a query-based custom check
// (which requires clang-tidy --experimental-custom-checks and a config file).
func AnyCustom(sel []Entry) bool {
	for _, e := range sel {
		if e.Custom {
			return true
		}
	}
	return false
}

// WithoutCustom returns the entries of sel that are not query-based custom
// checks, preserving order. Used to degrade gracefully when the clang-tidy on
// PATH is too old for --experimental-custom-checks: the built-in checks still
// run.
func WithoutCustom(sel []Entry) []Entry {
	out := make([]Entry, 0, len(sel))
	for _, e := range sel {
		if !e.Custom {
			out = append(out, e)
		}
	}
	return out
}

// ClangTidyConfig renders a .clang-tidy YAML enabling exactly sel: a Checks
// line for all entries and a CustomChecks block defining the query-based ones.
// Used when the selection contains custom checks — clang-tidy reads the custom
// definitions from the config and enables them via Checks.
func ClangTidyConfig(sel []Entry) string {
	var b strings.Builder
	b.WriteString("Checks: '" + TidyChecksArg(sel) + "'\n")
	var custom []Entry
	for _, e := range sel {
		if e.Custom {
			custom = append(custom, e)
		}
	}
	if len(custom) == 0 {
		return b.String()
	}
	b.WriteString("CustomChecks:\n")
	for _, e := range custom {
		name := strings.TrimPrefix(e.TidyName, "custom-")
		b.WriteString("  - Name: " + name + "\n")
		b.WriteString("    Query: |\n")
		for line := range strings.SplitSeq(e.Query, "\n") {
			b.WriteString("      " + line + "\n")
		}
		b.WriteString("    Diagnostic:\n")
		b.WriteString("      - BindName: " + e.Bind + "\n")
		b.WriteString("        Message: " + yamlQuote(e.Message) + "\n")
		b.WriteString("        Level: Warning\n")
	}
	return b.String()
}

// yamlQuote double-quotes a scalar and escapes embedded quotes/backslashes so
// messages with parentheses, colons, etc. round-trip safely.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
