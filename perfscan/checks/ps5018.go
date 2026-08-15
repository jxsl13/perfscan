package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5018 reports strings.Map(unicode.ToUpper, s) and
// strings.Map(unicode.ToLower, s) — the generic per-rune mapper driving
// the stdlib case table through a function pointer — and rewrites them
// to strings.ToUpper(s) / strings.ToLower(s), which carry an all-ASCII
// fast path and are DEFINED as exactly that Map call on every other
// input. This is the strings twin of PS5017 (the bare bytes->bytes
// call); the string-conversion round-trips through the bytes machinery
// are PS2017 (string(bytes.Map(f, []byte(s)))) and PS5010
// (string(bytes.ToUpper([]byte(s)))) — none of them matches the bare
// string->string call this check owns.
var PS5018 = register(&lint.Check{
	ID:       "PS5018",
	Category: "indirect",
	Slug:     "strings-map-unicode-case",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.Map(unicode.ToUpper, s) drives the case mapping through a per-rune indirect call; strings.ToUpper(s) is the same mapping with an all-ASCII fast path",
		Text: `strings.Map(unicode.ToUpper, s) pays, for EVERY rune of s, a
utf8 decode (the range loop), an INDIRECT call through the mapping
function pointer into unicode.ToUpper, and a re-encode into a builder
when anything changes. strings.ToUpper(s) computes the identical
mapping but first makes one linear byte scan: when s is all ASCII and
needs no case change — the dominant input for the extremely common
normalize-already-canonical uses (lowercasing header keys, identifiers,
email addresses that are already lower; uppercasing SCREAMING
constants) — it returns s ITSELF with zero per-rune work, about 2x
faster than Map's scan, which pays the indirect call for every rune
even when nothing changes. When an all-ASCII s does need changes,
strings.ToUpper runs a byte loop with no utf8 decode and no indirect
call; measured on go1.26 that is faster when changes are SPARSE but —
honestly — about 15-20% SLOWER than the Map spelling when most bytes
change case (unicode.ToUpper's own ASCII fast path makes Map's indirect
call cheap, while ToUpper's builder copies unchanged segments around
every cased byte). On any non-ASCII input strings.ToUpper is DEFINED as
return Map(unicode.ToUpper, s) (src/strings/strings.go), i.e. the very
call the user wrote — exact parity. The same holds for the ToLower
twin. Net: a clear win on the no-change and sparse-change ASCII paths
that dominate real normalization workloads, parity on non-ASCII, a
modest measured regression only on change-dense ASCII — and the
canonical stdlib spelling, which is where any future stdlib casing
optimization will land.

The rewrite is value-identical for every input. Non-ASCII path: the
same Map call executes, invalid/truncated UTF-8 included (both route
every byte >= 0x80 through the shared RuneError handling). All-ASCII
path: ranging over a byte below 0x80 yields that byte as a width-1
rune, and unicode.ToUpper / ToLower over ASCII changes only
'a'-'z' <-> 'A'-'Z' and returns every other ASCII rune unchanged
(never negative, so Map drops nothing) — exactly what the fast-path
byte loop computes. Strings are immutable, so value equality is the
whole story — there is no capacity or aliasing dimension to diverge
on; in fact both spellings even return s ITSELF (same backing bytes)
when nothing changes: Map's unchanged-input fast path and ToUpper /
ToLower's !hasLower / !hasUpper fast path both end in return s. There
is no context-sensitive special-casing on either side (Go's ToLower
has no final-sigma rule; ToUpper of ß is the simple 1:1 map, not SS),
because both route non-ASCII through the same per-rune
unicode.ToUpper/ToLower. The runtime differential test pins string
equality over exhaustive short inputs on an adversarial alphabet,
targeted Unicode case traps, and randomized inputs biased toward
invalid UTF-8.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level strings.Map (a
shadowed strings or a same-named method never matches) and the mapping
argument is exactly the standard library's package-level
unicode.ToUpper or unicode.ToLower (a wrapper func, a func variable, a
shadowed unicode, or a unicode.SpecialCase method value like
unicode.TurkishCase.ToUpper — which uses different tables — is
rejected by the receiver and package checks). unicode.ToTitle is
deliberately NOT matched: strings.ToTitle(s) is defined as exactly
Map(unicode.ToTitle, s) with no fast path, so that arm would be
readability-only, not a perf win. The haystack argument is kept
byte-verbatim in place — evaluated exactly once in both forms, any
type assignable to string stays legal — and the strings qualifier is
kept verbatim too, so an aliased strings import is reused as-is. The
fix deletes the mapping argument, which removes one unicode reference;
when the rewrites remove the file's last unicode reference the
orphaned unicode import is dropped as well (the runner never prunes
imports itself). A comment inside the deleted span, a cgo file that
would need that import surgery (its import block is never edited), or
a unicode import spelled more than once keeps the report advisory.`,
		Before: `out := strings.Map(unicode.ToUpper, s)`,
		After:  `out := strings.ToUpper(s)`,
		MeasuredWin: `BenchmarkPS5018 (Apple M2 Pro, go1.26): on an
already-uppercase 54-byte ASCII input (the no-change fast path that
dominates normalization workloads) strings.Map(unicode.ToUpper, s)
114.6 ns/op -> strings.ToUpper(s) 55.9 ns/op (~2x; both return s with
0 allocs, but Map still pays the per-rune indirect call to find that
out). On an ASCII input with sparse changes: 152.9 ns/op -> 120.5
ns/op. On non-ASCII input the two spellings execute the same Map code
and measure at parity (456 ns/op vs 459 ns/op). The one class where
the rewrite measures SLOWER on go1.26 is all-ASCII input where most
bytes change case: 138.6 ns/op -> 163.8 ns/op (~18% — ToUpper's
builder segment-copies around every cased byte, while unicode.
ToUpper's own ASCII fast path keeps Map's indirect call cheap); the
benchmarks pin all four classes rather than hiding that one.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5018",
		Doc:  "strings.Map(unicode.ToUpper, s) drives the case tables through a per-rune indirect call; strings.ToUpper(s) is the same mapping with an all-ASCII fast path",
		Run:  runPS5018,
	},
})

func ps5018Msg(fn string) string {
	return "strings.Map(unicode." + fn + ", s) pays a utf8 decode, an indirect call into the case tables, and a re-encode for every rune; strings." + fn + "(s) is the identical mapping (it IS this Map call on non-ASCII input) with an all-ASCII fast path that skips all of that"
}

func runPS5018(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site deletes exactly one unicode reference (the qualifier of
		// unicode.ToUpper/ToLower), and whether that orphans the unicode
		// import depends on ALL sites together (same per-file site
		// collection as PS5017/PS2017/PS5010).
		type site struct {
			call *ast.CallExpr
			fn   string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, fn, matched := ps5018Match(pass, call)
			if !matched {
				return true
			}
			fix := ps5018Fix(f, call, sel, fn)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim haystack span, whose edits never overlap this
			// site's edits.
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable site deletes exactly one unicode reference (the
		// package qualifier inside the deleted mapping argument; the kept
		// haystack's unicode references, if any, stay). When those are all
		// of the file's unicode references, the rewrites orphan the import
		// and the fix must drop it — an unused import is a compile error
		// and the runner never prunes imports itself.
		if fixable > 0 && pkgRefCount(pass, f, "unicode") == fixable {
			importEdit, ok := ps5017DropUnicode(f)
			if !ok {
				// cgo file (whose import block is never edited), or a
				// unicode import we cannot drop safely: keep every report
				// advisory.
				for i := range sites {
					sites[i].fix = nil
				}
			} else {
				// All fixes of a run are applied together, so only the
				// first fixable site carries the import edit (same
				// convention as PS5017/PS2017/PS5010).
				for i := range sites {
					if sites[i].fix != nil {
						sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdit)
						break
					}
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps5018Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5018Match matches call against strings.Map(unicode.ToUpper, s) or
// strings.Map(unicode.ToLower, s): the callee is the standard library's
// package-level strings.Map pinned by type information, and the first
// argument is (possibly parenthesized) exactly the standard library's
// package-level unicode.ToUpper or unicode.ToLower — a *types.Func from
// package unicode with a nil receiver, which rejects wrapper funcs, func
// variables, a shadowed unicode, and unicode.SpecialCase method values
// (whose signatures carry a receiver and whose tables differ). It
// returns the callee selector (whose Sel the fix renames) and the
// function name. unicode.ToTitle is deliberately not matched:
// strings.ToTitle has no fast path — it IS Map(unicode.ToTitle, s) — so
// that rewrite would be readability-only.
func ps5018Match(pass *analysis.Pass, call *ast.CallExpr) (sel *ast.SelectorExpr, fn string, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, "", false
	}
	mapObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || mapObj.Pkg() == nil || mapObj.Pkg().Path() != "strings" || mapObj.Name() != "Map" {
		return nil, "", false
	}
	if sig, isSig := mapObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, "", false
	}
	msel, isMSel := ps2108Unparen(call.Args[0]).(*ast.SelectorExpr)
	if !isMSel {
		return nil, "", false
	}
	fnObj, isFunc := pass.TypesInfo.Uses[msel.Sel].(*types.Func)
	if !isFunc || fnObj.Pkg() == nil || fnObj.Pkg().Path() != "unicode" {
		return nil, "", false
	}
	switch fnObj.Name() {
	case "ToUpper", "ToLower":
	default:
		return nil, "", false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		// A unicode.SpecialCase method value (unicode.TurkishCase.ToUpper)
		// uses different tables — never matched.
		return nil, "", false
	}
	return sel, fnObj.Name(), true
}

// ps5018Fix builds the strings.ToUpper(s)/ToLower(s) rewrite for one
// site, or nil when a guard fails and the report must stay advisory. Two
// edits: the callee's Sel identifier "Map" becomes the case function's
// name (the strings qualifier stays verbatim, so an aliased import is
// reused as-is), and the span from the mapping argument through the
// following comma is deleted. The haystack argument stays untouched in
// place, preserving its text and single evaluation. The import edit,
// when needed, is appended later, once per file.
func ps5018Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, fn string) *analysis.SuggestedFix {
	hay := call.Args[1]
	// The deleted span runs from the mapping argument to the haystack; a
	// comment there would be silently destroyed — advisory then. (The
	// other edit replaces a bare identifier, which cannot hold comments.)
	if ps2111CommentIn(f, call.Args[0].Pos(), hay.Pos()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + fn + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(fn)},
			{Pos: call.Args[0].Pos(), End: hay.Pos()},
		},
	}
}
