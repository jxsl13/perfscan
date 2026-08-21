package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3030 reports bytes.FieldsFunc(b, unicode.IsSpace) — the general
// splitter driven by an indirect predicate call per decoded rune — and
// rewrites it to bytes.Fields(b), which is DEFINED as exactly that call
// but front-loads a byte-table ASCII fast path: one pass over raw bytes
// with no rune decoding and no indirect calls on all-ASCII input. The
// bytes twin of PS5034.
var PS3030 = register(&lint.Check{
	ID:       "PS3030",
	Category: "indirect",
	Slug:     "bytes-fieldsfunc-isspace-to-fields",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.FieldsFunc(b, unicode.IsSpace) decodes and calls the predicate per rune; bytes.Fields(b) is the same split with a byte-table ASCII fast path",
		Text: `bytes.FieldsFunc always runs the general algorithm: it
utf8.DecodeRune-decodes EVERY rune of b, calls the predicate through a
function value (an indirect call the compiler cannot inline or
devirtualize) once per rune, appends span records, and finally slices
the fields out. bytes.Fields is documented — and implemented — as
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
input either — an input with any byte >= 0x80 takes the identical-by-
construction branch, and RuneError from a bad byte is not a space).
Both sides return three-index-capped subslices of the SAME backing
array — Fields' ASCII path emits s[fieldStart:i:i] /
s[fieldStart:len(s):len(s)] and FieldsFunc emits
s[span.start:span.end:span.end] — so every field's bytes, cap, AND
backing-array identity (the aliasing surface that matters for a
mutable []byte) agree. Both sides allocate the result EXACTLY
(make([][]byte, fieldCount)), so len, cap, and non-nil-ness of the
outer slice (both return an empty non-nil slice for nil, empty, and
all-space input) agree everywhere. b is evaluated exactly once in both
spellings, and neither side can panic. The runtime differential test
pins all of this — including per-field base-pointer identity — over
exhaustive short inputs (all byte combinations of ASCII spaces,
letters, NUL, UTF-8 lead/continuation bytes, and 0xFF — so NBSP/NEL
and truncated sequences arise at every position), targeted
Unicode-space and invalid-UTF-8 seeds, nil, and seeded-random long
inputs.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level bytes.FieldsFunc (a
shadowed bytes identifier or a same-named method never matches) and
the predicate argument is the BARE unicode.IsSpace selector pinned to
the package-level function in package unicode (an equivalent wrapper
literal func(r rune) bool { return unicode.IsSpace(r) }, a variable
holding unicode.IsSpace, any other predicate, and strings.FieldsFunc —
PS5034's territory — are all out of scope). Only the selected name
(FieldsFunc -> Fields) and the trailing ", unicode.IsSpace" are
edited; the b argument stays byte-verbatim in place, so aliased bytes
and unicode imports keep working. Fields lives in the same package, so
no import is ever added; when the rewrites remove the file's LAST
unicode reference the fix also drops the orphaned unicode import. A
comment inside the deleted ", unicode.IsSpace)" scaffolding, or an
orphaned unicode import in a cgo file (whose import block must never
be edited), keeps that report advisory.`,
		Before: `parts := bytes.FieldsFunc(b, unicode.IsSpace)`,
		After:  `parts := bytes.Fields(b)`,
		MeasuredWin: `BenchmarkPS3030 (a ~1 KB all-ASCII log line with 96
fields, plus the same line with one trailing NBSP forcing the non-ASCII
path, Apple M2 Pro, go1.26): ASCII input 3.3 µs/op, 5760 B/op,
3 allocs/op -> 1.3 µs/op, 2688 B/op, 1 alloc/op (~2.6x, and only the
exact result slice remains — no rune decode, no indirect predicate
call, and none of FieldsFunc's internal span-slice growth); non-ASCII
input 3.4 µs/op -> 3.8 µs/op, identical allocations (~13% extra for the
byte-counting prepass before Fields delegates to the IDENTICAL
FieldsFunc call — the bounded worst case, paid only when the input
actually contains a non-ASCII byte).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3030",
		Doc:  "bytes.FieldsFunc(b, unicode.IsSpace) runs the general per-rune splitter; bytes.Fields(b) is the identical split with a byte-table ASCII fast path",
		Run:  runPS3030,
	},
})

const ps3030Msg = "bytes.FieldsFunc(b, unicode.IsSpace) decodes every rune and calls the predicate indirectly per rune; bytes.Fields(b) is the identical split — Fields is defined as FieldsFunc(s, unicode.IsSpace) — with a byte-table ASCII fast path and no indirect calls"

func runPS3030(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide the import edit once per file: every
		// fixable site deletes exactly one unicode reference (the
		// predicate's qualifier), and whether that orphans the unicode
		// import depends on ALL sites together (same per-file site
		// collection as PS2012/PS5034).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !ps3030Match(pass, call) {
				return true
			}
			fix := ps3030Fix(f, call)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim b argument, whose edits never overlap this
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
				// convention as PS2012/PS5034).
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
				Message: ps3030Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3030Match reports whether call is bytes.FieldsFunc(b, p) with the
// callee pinned by type information to the standard library's
// package-level bytes.FieldsFunc and p exactly the (possibly
// parenthesized) bare selector for the package-level unicode.IsSpace.
// A shadowed bytes or unicode identifier, a same-named method or
// func-typed field, a wrapper literal, a variable holding
// unicode.IsSpace, any other predicate, and strings.FieldsFunc all fail
// the type pins and are not matched. b needs no constraint of its own:
// Fields' parameter type is identical to FieldsFunc's first parameter,
// so any b that compiles before compiles after.
func ps3030Match(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "FieldsFunc" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
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

// ps3030Fix builds the bytes.Fields(b) rewrite for one matched site, or
// nil when a guard fails and the report must stay advisory. Only the
// selected name and the scaffolding after the b argument are replaced;
// b itself stays untouched in place, preserving its text and single
// evaluation (same technique as PS5034). The orphaned-import edit, when
// needed, is appended later, once per file.
func ps3030Fix(f *ast.File, call *ast.CallExpr) *analysis.SuggestedFix {
	sel := call.Fun.(*ast.SelectorExpr)
	b := call.Args[0]
	// The deleted span is ", unicode.IsSpace)"; a comment there would be
	// silently destroyed — advisory then.
	if ps2111CommentIn(f, b.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with bytes.Fields(b)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Fields")},
			{Pos: b.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
