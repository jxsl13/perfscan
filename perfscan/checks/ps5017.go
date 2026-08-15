package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5017 reports bytes.Map(unicode.ToUpper, b) and
// bytes.Map(unicode.ToLower, b) — the generic per-rune mapper driving
// the stdlib case table through a function pointer — and rewrites them
// to bytes.ToUpper(b) / bytes.ToLower(b), which carry an all-ASCII fast
// path and are DEFINED as exactly that Map call on every other input.
// The string-conversion round-trips through the same machinery are
// PS2017 (string(bytes.Map(f, []byte(s)))) and PS5010
// (string(bytes.ToUpper([]byte(s)))); this is the bare bytes->bytes
// call with no string conversion, which neither of them matches.
var PS5017 = register(&lint.Check{
	ID:       "PS5017",
	Category: "indirect",
	Slug:     "bytes-map-unicode-case",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Map(unicode.ToUpper, b) pays a per-rune indirect call; bytes.ToUpper(b) is the same mapping with an all-ASCII fast path",
		Text: `bytes.Map(unicode.ToUpper, b) pays, for EVERY rune of b, a
utf8.DecodeRune, an INDIRECT call through the mapping function pointer
into unicode.ToUpper (which itself walks the caseRange tables), and a
utf8.AppendRune re-encode. bytes.ToUpper(b) computes the identical
mapping but first makes one linear scan: when b is all ASCII — the
dominant input in practice — it either memmove-copies b unchanged (when
there is nothing to case) or runs a tight byte loop that only tests
'a' <= c <= 'z', skipping the decode, the indirect call, the table
walk, and the re-encode entirely. On any non-ASCII input bytes.ToUpper
is DEFINED as return Map(unicode.ToUpper, s) (src/bytes/bytes.go), i.e.
the very call the user wrote — so the rewrite is never slower: strictly
less work on the ASCII path, textually the same work otherwise. The
same holds for the ToLower twin.

The rewrite is value-identical for every input. Non-ASCII path: the
same Map call executes, invalid/truncated UTF-8 included (RuneError
handling is the shared code). All-ASCII path: utf8.DecodeRune of a byte
below 0x80 yields that byte as a width-1 rune, and unicode.ToUpper /
ToLower over ASCII changes only 'a'-'z' <-> 'A'-'Z' and returns every
other ASCII rune unchanged (never negative, so Map drops nothing) —
exactly what the fast-path byte loop computes. Both spellings ALWAYS
return a freshly allocated slice that never aliases b, and both return
a NON-nil empty slice for empty (or nil) input, so mutate-the-result
semantics and nil-ness coincide — there is no strings-style
return-s-itself aliasing hazard on either side. The one observable
difference is the result's CAPACITY (Map sizes to exactly len(b); the
fast paths may round up to an allocator size class), which is the
accepted PS2112 class: the capacity of a freshly built slice is an
unspecified implementation detail no correct program relies on, and
aliasing with the input — absent on both sides — is what actually
matters. The runtime differential test pins content and nil-ness
equality over exhaustive short inputs on an adversarial alphabet,
targeted Unicode case traps, and randomized inputs biased toward
invalid UTF-8, and pins the non-aliasing on both sides.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level bytes.Map (a
shadowed bytes or a same-named method never matches) and the mapping
argument is exactly the standard library's package-level unicode.ToUpper
or unicode.ToLower (a wrapper func, a func variable, a shadowed
unicode, or a unicode.SpecialCase method value like
unicode.TurkishCase.ToUpper — which uses different tables — is
rejected by the receiver and package checks). unicode.ToTitle is
deliberately NOT matched: bytes.ToTitle(s) is defined as exactly
Map(unicode.ToTitle, s) with no fast path, so that arm would be
readability-only, not a perf win. The haystack argument is kept
byte-verbatim in place — evaluated exactly once in both forms, any
type assignable to []byte stays legal — and the bytes qualifier is
kept verbatim too, so an aliased bytes import is reused as-is. The fix
deletes the mapping argument, which removes one unicode reference;
when the rewrites remove the file's last unicode reference the
orphaned unicode import is dropped as well (the runner never prunes
imports itself). A comment inside the deleted span, a cgo file that
would need that import surgery (its import block is never edited), or
a unicode import spelled more than once keeps the report advisory.`,
		Before: `out := bytes.Map(unicode.ToUpper, b)`,
		After:  `out := bytes.ToUpper(b)`,
		MeasuredWin: `BenchmarkPS5017 (Apple M2 Pro, go1.26): on a 55-byte
all-ASCII mixed-case input, bytes.Map(unicode.ToUpper, b) 174.5 ns/op
-> bytes.ToUpper(b) 80.4 ns/op (~2.2x: the fast path cases bytes
without decoding runes or calling through the function pointer). On an
already-uppercase ASCII input the After is a plain copy: 164.6 ns/op
-> 83.7 ns/op (~2x). On non-ASCII input the two spellings execute the
same Map code and measure at parity (461 ns/op vs 455 ns/op) — the
rewrite is never slower.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5017",
		Doc:  "bytes.Map(unicode.ToUpper, b) drives the case tables through a per-rune indirect call; bytes.ToUpper(b) is the same mapping with an all-ASCII fast path",
		Run:  runPS5017,
	},
})

func ps5017Msg(fn string) string {
	return "bytes.Map(unicode." + fn + ", b) pays a utf8 decode, an indirect call into the case tables, and a re-encode for every rune; bytes." + fn + "(b) is the identical mapping (it IS this Map call on non-ASCII input) with an all-ASCII fast path that skips all of that"
}

func runPS5017(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site deletes exactly one unicode reference (the qualifier of
		// unicode.ToUpper/ToLower), and whether that orphans the unicode
		// import depends on ALL sites together (same per-file site
		// collection as PS2017/PS5010).
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
			sel, fn, matched := ps5017Match(pass, call)
			if !matched {
				return true
			}
			fix := ps5017Fix(f, call, sel, fn)
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
				// convention as PS2017/PS5010).
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
				Message: ps5017Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5017Match matches call against bytes.Map(unicode.ToUpper, b) or
// bytes.Map(unicode.ToLower, b): the callee is the standard library's
// package-level bytes.Map pinned by type information, and the first
// argument is (possibly parenthesized) exactly the standard library's
// package-level unicode.ToUpper or unicode.ToLower — a *types.Func from
// package unicode with a nil receiver, which rejects wrapper funcs, func
// variables, a shadowed unicode, and unicode.SpecialCase method values
// (whose signatures carry a receiver and whose tables differ). It
// returns the callee selector (whose Sel the fix renames) and the
// function name. unicode.ToTitle is deliberately not matched:
// bytes.ToTitle has no fast path — it IS Map(unicode.ToTitle, s) — so
// that rewrite would be readability-only.
func ps5017Match(pass *analysis.Pass, call *ast.CallExpr) (sel *ast.SelectorExpr, fn string, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, "", false
	}
	mapObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || mapObj.Pkg() == nil || mapObj.Pkg().Path() != "bytes" || mapObj.Name() != "Map" {
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

// ps5017Fix builds the bytes.ToUpper(b)/ToLower(b) rewrite for one site,
// or nil when a guard fails and the report must stay advisory. Two edits:
// the callee's Sel identifier "Map" becomes the case function's name
// (the bytes qualifier stays verbatim, so an aliased import is reused
// as-is), and the span from the mapping argument through the following
// comma is deleted. The haystack argument stays untouched in place,
// preserving its text and single evaluation. The import edit, when
// needed, is appended later, once per file.
func ps5017Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr, fn string) *analysis.SuggestedFix {
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

// ps5017DropUnicode returns the edit removing the file's unicode import,
// or ok=false when it must not or cannot be dropped: a cgo file (whose
// import block is never edited), no locatable spec, or the path imported
// under more than one spec (all of them would be orphaned; dropping just
// one would still leave a compile error — vanishingly rare, advisory).
func ps5017DropUnicode(f *ast.File) (edit analysis.TextEdit, ok bool) {
	if ps2110ImportsC(f) {
		return analysis.TextEdit{}, false
	}
	specs := 0
	for _, imp := range f.Imports {
		if imp.Path.Value == `"unicode"` {
			specs++
		}
	}
	if specs != 1 {
		return analysis.TextEdit{}, false
	}
	gd, i, spec, found := ps2012FindImport(f, `"unicode"`)
	if !found {
		return analysis.TextEdit{}, false
	}
	return ps2012RemoveSpec(gd, i, spec), true
}
