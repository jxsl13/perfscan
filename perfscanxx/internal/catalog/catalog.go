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

import (
	"slices"
	"strings"
)

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
	// Caveat, when non-empty, warns that clang-tidy's fix-it — though it
	// applies cleanly — can be UNSAFE to accept blindly for this check: the
	// rewrite may change observable behavior in cases clang-tidy's syntactic
	// analysis cannot see (e.g. reordering member reads past a lock, or
	// removing a deliberately-omitted noexcept). perfscanxx surfaces the fix
	// faithfully; the caveat tells a reviewer to eyeball `-diff` before `-fix`.
	// Only meaningful when HasFix is true. Printed by `-explain`.
	Caveat string

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
		Caveat: "a move op is sometimes left non-noexcept ON PURPOSE because a member " +
			"operation it performs can throw (e.g. a move-assignment that closes a file/handle " +
			"whose close may throw). Adding noexcept turns any such throw into std::terminate. " +
			"Confirm the move body cannot throw before -fix.",
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
		Caveat: "this is a trade-off, not a strict win. by-value + std::move pays off " +
			"only when callers pass RVALUES (moved in) of a nothrow-movable type; an LVALUE " +
			"caller that previously bound to the const& for free now pays a full COPY, so on " +
			"lvalue-heavy call sites it PESSIMIZES. It also changes how many copy/move " +
			"constructors run — observable if the type's copy/move has side effects " +
			"(refcounting, logging, allocation counters). Benchmark/review the call sites " +
			"before -fix.",
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
		Caveat: "the fix hoists the field reads into the member-initializer list, " +
			"which runs BEFORE the constructor body — so if the body acquires a lock " +
			"(or otherwise synchronizes) before reading those fields, the rewrite moves " +
			"the reads out from under it and can introduce a data race. Review -diff for " +
			"any lock_guard/mutex in the constructor body before -fix.",
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
	{
		// Advisory (clang-tidy emits no fix-it): a const local or const value
		// parameter that is returned (or thrown) can't be moved — its constness
		// defeats the implicit move, forcing a copy. The mechanical fix (drop
		// the const) is unsafe: the const may be load-bearing elsewhere in the
		// body, so it stays advisory. clang-tidy only fires where NRVO can't
		// already elide the copy (a const value parameter, or a const local
		// returned from a branch), so it flags REAL pessimizations, not copies
		// the compiler already elides. L2 (review): a deliberate const is
		// common, so this must never surface at the default level.
		ID: "PX3025", TidyName: "performance-no-automatic-move",
		Level: LevelStructured, Category: "moves",
		Title:  "a const local or value parameter that is returned can't be moved — its constness forces a copy where a move was possible",
		HasFix: false,
	},
	{
		// clang-tidy ships a working fix-it (verified end-to-end by
		// TestHasFixChecksActuallyApply): it defaults the destructor on its FIRST
		// (in-class) declaration and deletes the out-of-line `= default`
		// definition. A user-declared destructor — even one only defaulted out of
		// line — makes the type NON-trivially-destructible, which blocks trivial
		// relocation (a vector can no longer memcpy its elements on growth) and
		// forces a per-element destructor call the trivial case elides entirely.
		// Strict win, not a trade-off: the defaulted destructor does exactly what
		// the implicit one would, so behavior is unchanged — only the type's
		// trivial-destructibility trait is restored, so no caveat. L2 (review):
		// the fix flips an observable type trait (std::is_trivially_destructible),
		// so keep it off the default level even though the rewrite is safe.
		ID: "PX3026", TidyName: "performance-trivially-destructible",
		Level: LevelStructured, Category: "moves",
		Title:  "an out-of-line defaulted destructor makes the type non-trivially-destructible (blocks trivial relocation); default it on the first declaration",
		HasFix: true,
	},
	{
		// clang-tidy ships a working fix-it (verified end-to-end: it inserts
		// ` noexcept ` into the destructor declaration and the result compiles).
		// A destructor becomes implicitly noexcept(false) only when a base or
		// member destructor it runs is potentially-throwing; that propagates up
		// and blocks the same move/relocation optimizations the sibling noexcept
		// checks (PX3004/PX3006) target. Same family, same encoding.
		ID: "PX3027", TidyName: "performance-noexcept-destructor",
		Level: LevelStructured, Category: "moves",
		Title:  "a destructor left implicitly noexcept(false) because a base/member destructor can throw blocks move optimizations; mark it noexcept",
		HasFix: true,
		Caveat: "same trade-off as the noexcept-move-constructor check (PX3004): the " +
			"destructor is noexcept(false) because a base or member destructor it runs " +
			"can throw. Adding noexcept turns any such throw during destruction into " +
			"std::terminate. Confirm no subobject destructor actually throws before -fix.",
	},
	// Query-based custom check (ZERO compiled C++) — the C++ analog of the
	// Go linter's PS2101. Run via clang-tidy --experimental-custom-checks.
	{
		ID: "PX2101", TidyName: "custom-reserve-before-loop",
		Level: LevelStructured, Category: "allocation",
		Title:  "vector grown via push_back/emplace_back in a loop; reserve() the final size before it when known",
		HasFix: false,
		Custom: true,
		Bind:   "grow",
		// Any loop kind — forStmt alone missed range-for (the most common C++
		// loop) and while/do loops. isExpansionInMainFile keeps it off headers.
		// LIMITATION (why the message hedges): an AST matcher cannot reliably tell
		// whether the grown vector was already reserve()'d in a preceding sibling
		// statement — that needs data-flow, not a syntactic ancestor match. So the
		// check flags the PATTERN and fires whether or not a reserve is present; the
		// Title/Message are worded so an already-reserved loop reads as a
		// "confirm you reserved" nudge, not a false "you forgot to reserve" claim.
		Query: `match cxxMemberCallExpr(isExpansionInMainFile(), ` +
			`callee(cxxMethodDecl(hasAnyName("push_back", "emplace_back"))), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("grow")`,
		Message: "vector grown via push_back/emplace_back inside a loop; if the final size is known, reserve() it before the loop to avoid repeated reallocation — the query flags the pattern and cannot see whether a reserve is already present (perfscanxx PS2101 analog, query-based, no auto-fix)",
	},
	{
		ID: "PX2102", TidyName: "custom-pessimizing-move",
		Level: LevelStructured, Category: "moves",
		Title:  "return std::move(local) blocks copy/move elision (NRVO); return the local directly",
		HasFix: false,
		Custom: true,
		Bind:   "mv",
		// isExpansionInMainFile keeps it off headers (inline/template returns).
		// unless(parmVarDecl()): NRVO applies only to a named LOCAL, never to a
		// by-value PARAMETER (copy elision is barred for parameters, and
		// `return param;` already implicit-moves), so `return std::move(param)`
		// is redundant-but-harmless, NOT an NRVO pessimization. Excluding
		// parameters keeps this off that false positive — parameters have local
		// storage, so hasLocalStorage() alone would match them.
		Query: `match returnStmt(isExpansionInMainFile(), hasReturnValue(ignoringParenImpCasts(` +
			`cxxConstructExpr(hasArgument(0, ignoringParenImpCasts(` +
			`callExpr(callee(functionDecl(hasName("::std::move"))), ` +
			`hasArgument(0, declRefExpr(to(varDecl(hasLocalStorage(), unless(parmVarDecl())))))).bind("mv")))))))`,
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
		// hasAutomaticStorageDuration(): a `static`/`thread_local` regex in a loop
		// is initialized ONCE (static-local init) — `static const std::regex re`
		// inside the function is in fact the idiomatic hoist — so it does NOT
		// recompile per iteration and must be excluded; only an automatic
		// (per-iteration) variable is the anti-pattern.
		Query: `match varDecl(isExpansionInMainFile(), hasAutomaticStorageDuration(), ` +
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
		// hasAutomaticStorageDuration(): a static/thread_local stream is
		// constructed ONCE, so it does not reallocate per iteration — only an
		// automatic (per-iteration) variable is the anti-pattern (see PX2104).
		Query: `match varDecl(isExpansionInMainFile(), hasAutomaticStorageDuration(), ` +
			`hasType(hasCanonicalType(recordType(hasDeclaration(cxxRecordDecl(matchesName("::std::basic_(o|i)?stringstream")))))), ` +
			`hasAncestor(stmt(anyOf(forStmt(), cxxForRangeStmt(), whileStmt(), doStmt())))).bind("ss")`,
		Message: "std::stringstream constructed inside a loop heap-allocates a new buffer (and re-imbues the locale) every iteration; hoist the stream out of the loop and reset it with .str(\"\") each pass (query-based, no auto-fix)",
	},
	{
		ID: "PX2107", TidyName: "custom-pow-const-exponent",
		Level: LevelStructured, Category: "algorithms",
		Title:  "std::pow with a constant exponent pays a full libm call where a couple of multiplies (or std::sqrt) would do",
		HasFix: false,
		Custom: true,
		Bind:   "pow",
		// pow/powf/powl(x, <literal>) — a compile-time-constant exponent. pow() is
		// a general transcendental routine (tens of ns); for a small integer power
		// x*x / x*x*x is a multiply or two, and pow(x, 0.5) is std::sqrt(x).
		// clang-tidy ships no equivalent check. hasAnyName also catches the float
		// and long-double variants (powf/powl) numeric C/C++ code uses.
		// ignoringImpCasts sees through the int-literal -> double conversion so
		// pow(x, 2) matches as well as pow(x, 2.0). The unless(equals 0/1) excludes
		// the NON-actionable integer exponents where "multiply directly" is wrong
		// advice: pow(x, 0) is 1 and pow(x, 1) is x (corpus finding on abseil).
		// NO auto-fix, deliberately: the rewrite x*x evaluates the base TWICE
		// (unsafe if it has side effects, e.g. pow(f(), 2)), and the right form
		// depends on the exponent (multiply vs sqrt) — a human call.
		Query: `match callExpr(isExpansionInMainFile(), callee(functionDecl(hasAnyName("pow", "powf", "powl"))), ` +
			`hasArgument(1, ignoringImpCasts(anyOf(integerLiteral(unless(anyOf(equals(0), equals(1)))), floatLiteral())))).bind("pow")`,
		Message: "std::pow with a constant exponent pays a full libm call; for a small integer power multiply directly (x*x, x*x*x) and for pow(x, 0.5) use std::sqrt(x) — mind that x*x evaluates the base twice, so hoist it first if it has side effects (query-based, no auto-fix)",
	},
	{
		ID: "PX2108", TidyName: "custom-vector-bool",
		Level: LevelAggressive, Category: "containers",
		Title:  "std::vector<bool> is a space-optimized bitfield, not a real container — its proxy references and missing data() silently break generic code",
		HasFix: false,
		Custom: true,
		Bind:   "vb",
		// A variable or field (NOT a parameter — a by-value vector<bool> parameter is
		// a pass-through whose storage choice belongs to the caller's declaration
		// site, so flagging it too would double-report) whose canonical type is the
		// std::vector<bool> specialization. hasCanonicalType sees through typedef
		// aliases (`using Mask = std::vector<bool>;`). isExpansionInMainFile keeps it
		// off the standard library's own internal <vector> instantiations. The C++
		// analog is well known (Effective STL Item 18); clang-tidy ships no
		// equivalent. L3/aggressive because the bit-packing is sometimes chosen
		// deliberately (a memory-constrained bitset), so it must not surface below
		// the aggressive tier. NO auto-fix, deliberately: the right replacement
		// depends on intent — std::vector<char>/std::uint8_t for a real bool
		// container, std::bitset or boost::dynamic_bitset for a deliberate bitfield.
		Query: `match valueDecl(isExpansionInMainFile(), unless(parmVarDecl()), anyOf(varDecl(), fieldDecl()), ` +
			`hasType(hasCanonicalType(recordType(hasDeclaration(classTemplateSpecializationDecl(hasName("::std::vector"), ` +
			`hasTemplateArgument(0, refersToType(booleanType())))))))).bind("vb")`,
		Message: "std::vector<bool> is a space-optimized bitfield, not a real container: operator[] returns a proxy object (not bool&), it has no data() and does not model Container, so it silently breaks generic code and pessimizes element access (query-based, no auto-fix)",
	},
	{
		ID: "PX2109", TidyName: "custom-std-list",
		Level: LevelAggressive, Category: "containers",
		Title:  "std::list/std::forward_list is a node-per-element linked list — poor cache locality and an allocation per element; std::vector is usually faster",
		HasFix: false,
		Custom: true,
		Bind:   "lst",
		// A variable or field (NOT a parameter — a by-value list parameter is a
		// pass-through whose storage choice belongs to the caller's declaration site)
		// whose canonical type is a std::list or std::forward_list specialization.
		// hasCanonicalType sees through typedef aliases (`using L = std::list<int>;`).
		// isExpansionInMainFile keeps it off the standard library's own <list>
		// instantiations. clang-tidy ships no equivalent. L3/aggressive because a
		// linked list is occasionally the right call (O(1) splice, or reference/
		// iterator stability across insertions and erasures), so it must not surface
		// below the aggressive tier. NO auto-fix, deliberately: swapping to
		// std::vector changes iterator/reference-invalidation and splice semantics —
		// a human must confirm the code does not rely on them.
		Query: `match valueDecl(isExpansionInMainFile(), unless(parmVarDecl()), anyOf(varDecl(), fieldDecl()), ` +
			`hasType(hasCanonicalType(recordType(hasDeclaration(cxxRecordDecl(hasAnyName("::std::list", "::std::forward_list"))))))).bind("lst")`,
		Message: "std::list/std::forward_list allocates a separate node per element and scatters them in memory, so traversal misses cache on every step and each insert allocates; std::vector (or std::deque) is usually faster — prefer it unless you specifically need O(1) splice or reference/iterator stability across insert/erase (query-based, no auto-fix)",
	},
	{
		ID: "PX2110", TidyName: "custom-count-for-existence",
		Level: LevelStructured, Category: "algorithms",
		Title:  "std::count/std::count_if(...) compared to answer existence scans the whole range; std::find/std::any_of stops at the first match",
		HasFix: false,
		Custom: true,
		Bind:   "cnt",
		// A std::count OR std::count_if algorithm call compared to 0/1 purely to
		// test existence: count(...) > 0, count(...) != 0, or count(...) >= 1.
		// std::count/count_if always walk the ENTIRE range, so using one for a
		// boolean "is there any" does N comparisons where std::find / std::find_if
		// (or C++20 std::ranges::any_of / C++23 std::ranges::contains) stops at the
		// first hit. The operator/literal pairing is exact so that count(...) > 1 (a
		// genuine "more than one" test that NEEDS the count) and count(...) == k are
		// NOT flagged; a member .count() on a set/map (its own O(log n) existence
		// primitive) is excluded because the callee is the free ::std::count/count_if
		// function, not a method. isExpansionInMainFile keeps it off library headers.
		// clang-tidy ships no equivalent (the C++ analog of perfscan's PS5104
		// strings.Count>0 -> Contains). NO auto-fix: the right replacement (find /
		// any_of / contains / binary_search) is a human call.
		Query: `match binaryOperator(isExpansionInMainFile(), ` +
			`hasEitherOperand(ignoringImpCasts(callExpr(callee(functionDecl(hasAnyName("::std::count", "::std::count_if")))))), ` +
			`anyOf(allOf(hasAnyOperatorName(">", "!="), hasEitherOperand(ignoringImpCasts(integerLiteral(equals(0))))), ` +
			`allOf(hasOperatorName(">="), hasEitherOperand(ignoringImpCasts(integerLiteral(equals(1))))))).bind("cnt")`,
		Message: "std::count/std::count_if scans the entire range to answer an existence question; std::find / std::find_if (or, C++20, std::ranges::any_of / C++23 std::ranges::contains) stops at the first match — and for a sorted range std::binary_search is O(log n) (query-based, no auto-fix)",
	},
	{
		ID: "PX2111", TidyName: "custom-map-double-lookup",
		Level: LevelStructured, Category: "containers",
		Title:  "an associative container is looked up twice — m.count(k)/m.find(k)!=m.end() then m[k]/m.at(k) on the same key; find() returns an iterator you can test and reuse",
		HasFix: false,
		Custom: true,
		Bind:   "dl",
		// The classic double lookup: `if (m.count(k)) { ... m[k] ... }` OR
		// `if (m.find(k) != m.end()) { ... m[k] ... }` hashes/compares the key TWICE —
		// once for the existence check, again for operator[] — where a single find()
		// answers both: `auto it = m.find(k); if (it != m.end()) use(it->second);`.
		// PRECISION: the map object and the key must be the SAME declared variable in
		// both the condition and the body — equalsBoundNode on the map's varDecl and on
		// the key's valueDecl — so `if (m.count(a)) use(m[b])` (different keys) is NOT
		// flagged. The condition is either a MEMBER count() on the object (so the free
		// std::count algorithm, PX2110, can never match here) or `m.find(k) != m.end()`
		// where the != is the iterators' overloaded operator (cxxOperatorCallExpr) and
		// the end() is on the SAME map; the == m.end() ABSENCE form is deliberately not
		// matched (there m[k] is an insert, not a redundant lookup — a try_emplace case).
		// The body access is operator[] OR .at() on the same map and key.
		// forEachDescendant sees through the ExprWithCleanups wrapping the
		// iterator-temporary comparison; ignoringParenImpCasts through the const-ref key
		// binding. isExpansionInMainFile keeps it off library headers. clang-tidy ships
		// no equivalent. (The C++20 contains() condition form is not yet matched — a
		// libc++ heterogeneous-lookup wrinkle on the key argument.) NO auto-fix: the
		// rewrite restructures the if around a find() iterator — a human call.
		Query: `match ifStmt(isExpansionInMainFile(), ` +
			`hasCondition(forEachDescendant(anyOf(` +
			`cxxMemberCallExpr(on(declRefExpr(to(varDecl().bind("m")))), callee(cxxMethodDecl(hasName("count"))), ` +
			`hasArgument(0, ignoringParenImpCasts(declRefExpr(to(valueDecl().bind("key")))))), ` +
			`cxxOperatorCallExpr(hasOverloadedOperatorName("!="), ` +
			`hasArgument(0, ignoringParenImpCasts(cxxMemberCallExpr(on(declRefExpr(to(varDecl().bind("m")))), ` +
			`callee(cxxMethodDecl(hasName("find"))), hasArgument(0, ignoringParenImpCasts(declRefExpr(to(valueDecl().bind("key")))))))), ` +
			`hasArgument(1, ignoringParenImpCasts(cxxMemberCallExpr(on(declRefExpr(to(varDecl(equalsBoundNode("m"))))), ` +
			`callee(cxxMethodDecl(hasName("end")))))))))), ` +
			`hasThen(forEachDescendant(anyOf(cxxOperatorCallExpr(hasOverloadedOperatorName("[]"), ` +
			`hasArgument(0, declRefExpr(to(varDecl(equalsBoundNode("m"))))), ` +
			`hasArgument(1, ignoringParenImpCasts(declRefExpr(to(valueDecl(equalsBoundNode("key"))))))), ` +
			`cxxMemberCallExpr(on(declRefExpr(to(varDecl(equalsBoundNode("m"))))), callee(cxxMethodDecl(hasName("at"))), ` +
			`hasArgument(0, ignoringParenImpCasts(declRefExpr(to(valueDecl(equalsBoundNode("key"))))))))))).bind("dl")`,
		Message: "the key is looked up twice on this map — the condition (m.count(k) or m.find(k) != m.end()) and m[k]/m.at(k) in the body hash/compare the same key again; hold the iterator instead: auto it = m.find(k); if (it != m.end()) use(it->second); (query-based, no auto-fix)",
	},
	{
		ID: "PX2112", TidyName: "custom-redundant-move-temporary",
		Level: LevelStructured, Category: "moves",
		Title:  "std::move on a temporary (prvalue) is redundant and turns it into an xvalue, blocking the copy elision that would construct the value in place; drop the std::move",
		HasFix: false,
		Custom: true,
		Bind:   "mv",
		// std::move applied to a TEMPORARY (a prvalue — a by-value function/method
		// call or a constructor temporary) in ANY position: `return std::move(f())`,
		// `auto x = std::move(f())`, `sink(std::move(f()))`, `v.push_back(std::move(
		// f()))`. The prvalue is already an rvalue, so the std::move is always
		// redundant; worse, it makes the expression an xvalue, defeating the
		// (guaranteed, in a return/initializer) copy elision that would have
		// constructed the value in place — a pessimization (an extra move) wherever
		// elision applied. The argument is matched as a cxxBindTemporaryExpr: a
		// materialized prvalue temporary whose type has a non-trivial destructor (the
		// resource-owning, move-relevant types — std::string/vector/unique_ptr/…).
		// This is the distinguishing signal — an lvalue-reference argument
		// (std::move(getRef()), a REAL move) has no bound temporary and is NOT
		// flagged, and moving a NAMED LOCAL is PX2102's job (a declRefExpr, no
		// temporary). isExpansionInMainFile keeps it off library headers. clang-tidy's
		// performance-move-const-arg does NOT catch this (verified). NO auto-fix:
		// dropping the std::move wrapper is a trivial but human edit.
		Query: `match callExpr(isExpansionInMainFile(), callee(functionDecl(hasName("::std::move"))), ` +
			`hasArgument(0, cxxBindTemporaryExpr())).bind("mv")`,
		Message: "std::move on a temporary here is redundant — a prvalue is already an rvalue — and turns it into an xvalue, defeating the copy elision that would construct the value in place (a pessimization in a return or initializer); drop the std::move (query-based, no auto-fix)",
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
	// Return in ascending PX-id order so -list/-json/-sarif and every selector
	// render checks predictably. The source literal groups related checks, which
	// leaves some ids out of numeric order (e.g. PX2002 after PX2005). Every id
	// is "PX"+4 digits, so a lexicographic compare is a numeric compare.
	slices.SortFunc(out, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
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
