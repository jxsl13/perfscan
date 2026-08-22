package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5034 reports strings.FieldsFunc(s, unicode.IsSpace) — the general
// splitter driven by an indirect predicate call per decoded rune — and
// rewrites it to strings.Fields(s), which is DEFINED as exactly that
// call but front-loads a byte-table ASCII fast path: one pass over raw
// bytes with no rune decoding and no indirect calls on all-ASCII input.
var PS5034 = register(&lint.Check{
	ID:       "PS5034",
	Category: "indirect",
	Slug:     "fieldsfunc-isspace-to-fields",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.FieldsFunc(s, unicode.IsSpace) decodes and calls the predicate per rune; strings.Fields(s) is the same split with a byte-table ASCII fast path",
		Text: `strings.FieldsFunc always runs the general algorithm: it ranges
over the string decoding EVERY rune, calls the predicate through a
function value (an indirect call the compiler cannot inline or
devirtualize) once per rune, appends span records, and finally slices
the fields out. strings.Fields is documented — and implemented — as
FieldsFunc(s, unicode.IsSpace) with an ASCII fast path in front: a
single pass over the RAW BYTES through the six-entry asciiSpace table
(no utf8 decode, no indirect call, no per-rune branch on a function
value) that counts the fields and slices them out directly. Only when
that pass sees a byte >= utf8.RuneSelf does Fields delegate to the
literal call FieldsFunc(s, unicode.IsSpace). On the dominant
mostly-ASCII input the rewrite removes one indirect call plus one rune
decode per rune; on non-ASCII input it costs one extra byte-counting
pass before running the identical code — a small fraction of the
general algorithm's per-rune work, bounded by the benchmark pair. The
ASCII path also skips FieldsFunc's internal span-record bookkeeping,
whose append growth allocates scratch beyond the result slice once the
input holds more than 32 fields.

The rewrite is bit-identical for every input. The non-ASCII branch of
Fields RETURNS FieldsFunc(s, unicode.IsSpace) — identical by
construction. The ASCII branch produces the same non-empty maximal
runs of non-space bytes in the same order, where "space" is the
asciiSpace table ('\t', '\n', '\v', '\f', '\r', ' ') — exactly
unicode.IsSpace restricted to bytes below utf8.RuneSelf, and the branch
is only taken when every byte is below utf8.RuneSelf (which is also
necessarily valid UTF-8, so the two sides cannot disagree on malformed
input either — a file with any byte >= 0x80 takes the identical-by-
construction branch, and RuneError from a bad byte is not a space).
Both sides allocate the result EXACTLY: Fields' ASCII path does
make([]string, n) and FieldsFunc does make([]string, len(spans)) with
n == len(spans) == the field count, so len, cap, non-nil-ness (both
return an empty non-nil slice for ""/all-space input), element order,
and element bytes agree everywhere. Every element is a substring of s
(strings are immutable — no aliasing surface), s is evaluated exactly
once in both spellings, and neither side can panic. The runtime
differential test pins this over exhaustive short inputs (all byte
combinations of ASCII spaces, letters, NUL, UTF-8 lead/continuation
bytes, and 0xFF — so NBSP/NEL and truncated sequences arise at every
position), targeted Unicode-space and invalid-UTF-8 seeds, and
seeded-random long inputs.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level strings.FieldsFunc
(a shadowed strings identifier or a same-named method never matches)
and the predicate argument is the BARE unicode.IsSpace selector pinned
to the package-level function in package unicode (an equivalent wrapper
literal func(r rune) bool { return unicode.IsSpace(r) }, a variable
holding unicode.IsSpace, any other predicate, and bytes.FieldsFunc are
all out of scope). Only the selected name (FieldsFunc -> Fields) and
the trailing ", unicode.IsSpace" are edited; the s argument stays
byte-verbatim in place, so aliased strings and unicode imports keep
working. Fields lives in the same package, so no import is ever added;
when the rewrites remove the file's LAST unicode reference the fix
also drops the orphaned unicode import. A comment inside the deleted
", unicode.IsSpace)" scaffolding, or an orphaned unicode import in a
cgo file (whose import block must never be edited), keeps that report
advisory.`,
		Before: `parts := strings.FieldsFunc(s, unicode.IsSpace)`,
		After:  `parts := strings.Fields(s)`,
		MeasuredWin: `BenchmarkPS5034 (a ~1 KB all-ASCII log line with 96
fields, plus the same line with one trailing NBSP forcing the non-ASCII
path, Apple M2 Pro, go1.26): ASCII input 3.3 µs/op, 4864 B/op,
3 allocs/op -> 1.2 µs/op, 1792 B/op, 1 alloc/op (~2.7x, and only the
exact result slice remains — no rune decode, no indirect predicate
call, and none of FieldsFunc's internal span-slice growth); non-ASCII
input 3.3 µs/op -> 3.7 µs/op, identical allocations (~14% extra for the
byte-counting prepass before Fields delegates to the IDENTICAL
FieldsFunc call — the bounded worst case, paid only when the input
actually contains a non-ASCII byte).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5034",
		Doc:  "strings.FieldsFunc(s, unicode.IsSpace) runs the general per-rune splitter; strings.Fields(s) is the identical split with a byte-table ASCII fast path",
		Run:  runPS5034,
	},
})

const ps5034Msg = "strings.FieldsFunc(s, unicode.IsSpace) decodes every rune and calls the predicate indirectly per rune; strings.Fields(s) is the identical split — Fields is defined as FieldsFunc(s, unicode.IsSpace) — with a byte-table ASCII fast path and no indirect calls"

func runPS5034(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide the import edit once per file: every
		// fixable site deletes exactly one unicode reference (the
		// predicate's qualifier), and whether that orphans the unicode
		// import depends on ALL sites together (same per-file site
		// collection as PS2012/PS2118).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !ps5034Match(pass, call) {
				return true
			}
			fix := ps5034Fix(f, call)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim s argument, whose edits never overlap this
			// site's selector/tail edits.
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable site's deleted span holds exactly one unicode
		// reference (the predicate's qualifier); when those are ALL of
		// the file's unicode references, the rewrites orphan the import
		// and the fix must drop it (the runner never prunes imports
		// itself). An advisory site keeps its reference, so its
		// presence correctly blocks the removal.
		if fixable > 0 && pkgRefCount(pass, f, "unicode") == fixable {
			gd, i, spec, found := ps2012FindImport(f, `"unicode"`)
			if ps2110ImportsC(f) || !found {
				// cgo file needing import surgery, or a unicode spec we
				// could not locate: keep every report advisory.
				for i := range sites {
					sites[i].fix = nil
				}
			} else {
				// All fixes of a run are applied together, so only the
				// first fixable site carries the import edit (same
				// convention as PS2012/PS2110).
				edit := ps2012RemoveSpec(gd, i, spec)
				for i := range sites {
					if sites[i].fix != nil {
						sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, edit)
						break
					}
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: ps5034Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5034Match reports whether call is strings.FieldsFunc(s, p) with the
// callee pinned by type information to the standard library's
// package-level strings.FieldsFunc and p exactly the (possibly
// parenthesized) bare selector for the package-level unicode.IsSpace.
// A shadowed strings or unicode identifier, a same-named method or
// func-typed field, a wrapper literal, a variable holding
// unicode.IsSpace, any other predicate, and bytes.FieldsFunc all fail
// the type pins and are not matched. s needs no constraint of its own:
// Fields' parameter type is identical to FieldsFunc's first parameter,
// so any s that compiles before compiles after.
func ps5034Match(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "FieldsFunc" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return false
	}
	if sig, ok := fn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return false
	}
	pred, ok := ps2108Unparen(call.Args[1]).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pfn, ok := pass.TypesInfo.Uses[pred.Sel].(*types.Func)
	if !ok || pfn.Name() != "IsSpace" || pfn.Pkg() == nil || pfn.Pkg().Path() != "unicode" {
		return false
	}
	if sig, ok := pfn.Type().(*types.Signature); !ok || sig.Recv() != nil {
		return false
	}
	return true
}

// ps5034Fix builds the strings.Fields(s) rewrite for one matched site,
// or nil when a guard fails and the report must stay advisory. Only the
// selected name and the scaffolding after the s argument are replaced;
// s itself stays untouched in place, preserving its text and single
// evaluation (same technique as PS2012/PS5031). The orphaned-import
// edit, when needed, is appended later, once per file.
func ps5034Fix(f *ast.File, call *ast.CallExpr) *analysis.SuggestedFix {
	sel := call.Fun.(*ast.SelectorExpr)
	s := call.Args[0]
	// The deleted span is ", unicode.IsSpace)"; a comment there would be
	// silently destroyed — advisory then.
	if ps2111CommentIn(f, s.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with strings.Fields(s)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Fields")},
			{Pos: s.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
