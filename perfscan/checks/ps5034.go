package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5034 reports strings.TrimFunc(s, unicode.IsSpace) — the generic
// rune-at-a-time trimmer driving the stdlib white-space classifier
// through a function pointer — and rewrites it to strings.TrimSpace(s),
// which is DEFINED as exactly that trim and adds an all-ASCII fast path
// that inspects boundary bytes by table lookup. The predicate-dropping
// twin of PS5018 (strings.Map(unicode.ToUpper, s) -> strings.ToUpper(s));
// the string(bytes.TrimFunc([]byte(s), f)) round-trip is PS2016, which
// keeps the TrimFunc spelling and never overlaps a site with this check.
var PS5034 = register(&lint.Check{
	ID:       "PS5034",
	Category: "indirect",
	Slug:     "trimfunc-isspace-to-trimspace",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "strings.TrimFunc(s, unicode.IsSpace) decodes every boundary rune through an indirect call; strings.TrimSpace(s) is the same trim with an ASCII byte-table fast path",
		Text: `strings.TrimFunc(s, unicode.IsSpace) pays, for EVERY leading and
trailing rune it examines, a utf8.DecodeRuneInString /
DecodeLastRuneInString and an INDIRECT call through the predicate
function pointer into unicode.IsSpace — a call the compiler can neither
inline nor devirtualize, on top of IsSpace's own range-table logic for
non-Latin-1 runes. strings.TrimSpace(s) computes the identical trim but
scans the boundaries with a 256-entry byte-table lookup (asciiSpace)
first: for the overwhelmingly common mostly-ASCII input it does
strictly less work per boundary byte — no rune decode, no indirect
call, no table walk — and it only falls back to the rune path when it
actually meets a byte >= utf8.RuneSelf.

The rewrite is bit-identical for every input. strings.TrimSpace is
DEFINED as the trim over unicode.IsSpace, and its implementation
literally delegates to TrimFunc(s[lo:], unicode.IsSpace) (resp.
TrimRightFunc) the moment its ASCII scan meets a non-ASCII byte — the
very call the user wrote. On the fast path, asciiSpace marks exactly
{'\t', '\n', '\v', '\f', '\r', ' '}, which equals unicode.IsSpace
restricted to single bytes below utf8.RuneSelf — the other two Latin-1
spaces, U+0085 (NEL) and U+00A0 (NBSP), are multi-byte in UTF-8, so as
encoded runes they reach the shared rune path in both spellings, and as
RAW bytes (invalid UTF-8) both spellings decode utf8.RuneError, which
is not a space. Both forms return a substring of s with identical
start/stop bounds, and a string result has no capacity or aliasing
surface, so value equality is the whole observable story (the all-space
input yields the empty string on both sides). s is evaluated exactly
once in both forms; the discarded predicate is a pure package-function
reference with no side effects and no evaluation of its own. The
runtime differential test pins byte equality over exhaustive short
inputs on an adversarial alphabet (ASCII spaces, NBSP/NEL bytes both
complete and truncated, bare continuation bytes, 0xFF), targeted
Unicode white-space seeds, and randomized inputs biased toward
boundary white space and invalid UTF-8.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level strings.TrimFunc (a
shadowed strings or a same-named method never matches) and the
predicate argument is exactly the standard library's package-level
unicode.IsSpace (a wrapper func, a func-typed variable holding
unicode.IsSpace, a shadowed unicode, or any other predicate is
rejected by the package and receiver checks). Only the full TrimFunc
maps: strings.TrimLeftFunc and strings.TrimRightFunc have no
TrimLeftSpace/TrimRightSpace counterpart and are never matched.
bytes.TrimFunc is out of scope — a []byte result has a capacity and
aliasing surface this string-only argument does not cover. The source
argument is kept byte-verbatim in place — same text, same single
evaluation, and any type assignable to string (a named string type
included) stays legal because TrimSpace takes the identical parameter
type — and the strings qualifier is kept verbatim too, so an aliased
strings import is reused as-is. The fix deletes the predicate
argument, which removes one unicode reference; when the rewrites
remove the file's last unicode reference the orphaned unicode import
is dropped as well (the runner never prunes imports itself). A comment
inside the deleted span, a cgo file that would need that import
surgery (its import block is never edited), or a unicode import
spelled more than once keeps the report advisory.

One adjacency, not an overlap: PS2016 rewrites
string(bytes.TrimFunc([]byte(s), unicode.IsSpace)) to
strings.TrimFunc(s, unicode.IsSpace), which is this check's Before
shape — the two checks fire on disjoint sites (bytes.TrimFunc there,
strings.TrimFunc here), and a -fix pass over PS2016's output converges
to strings.TrimSpace(s) on the next run with no oscillation.`,
		Before: `out := strings.TrimFunc(s, unicode.IsSpace)`,
		After:  `out := strings.TrimSpace(s)`,
		MeasuredWin: `BenchmarkPS5034 (Apple M2 Pro, go1.26): a 51-byte
ASCII log line padded with " \t\r\n  " on both sides drops from
36.4 ns/op to 6.9 ns/op (~5.3x — the boundary runes are classified by
byte-table lookup instead of a decode plus an indirect IsSpace call
each). An already-trimmed ASCII line drops from 7.9 ns/op to 2.5 ns/op
(~3.2x — TrimSpace inspects one byte per end, TrimFunc still decodes
and indirect-calls at both ends). An NBSP-padded input measures at
parity (29.8 ns/op vs 31.5 ns/op, ~5% over the delegation) because
TrimSpace immediately hands off to the very TrimFunc call the user
wrote. 0 B/op and 0 allocs/op on every side — both spellings return a
substring.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5034",
		Doc:  "strings.TrimFunc(s, unicode.IsSpace) decodes every boundary rune and calls the predicate through a function pointer; strings.TrimSpace(s) is the identical trim with an ASCII byte-table fast path",
		Run:  runPS5034,
	},
})

const ps5034Msg = "strings.TrimFunc(s, unicode.IsSpace) decodes every boundary rune and calls unicode.IsSpace through a function pointer; strings.TrimSpace(s) is the identical trim (it delegates to this very TrimFunc call past its fast path) with an ASCII byte-table fast path"

func runPS5034(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site deletes exactly one unicode reference (the qualifier of
		// unicode.IsSpace), and whether that orphans the unicode import
		// depends on ALL sites together (same per-file site collection as
		// PS5017/PS5018).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, matched := ps5034Match(pass, call)
			if !matched {
				return true
			}
			fix := ps5034Fix(f, call, sel)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim source-argument span, whose edits never overlap
			// this site's edits.
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable site deletes exactly one unicode reference (the
		// package qualifier inside the deleted predicate argument; the
		// kept source argument's unicode references, if any, stay). When
		// those are all of the file's unicode references, the rewrites
		// orphan the import and the fix must drop it — an unused import
		// is a compile error and the runner never prunes imports itself.
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
				// convention as PS5017/PS5018).
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

// ps5034Match matches call against strings.TrimFunc(s, unicode.IsSpace):
// the callee is the standard library's package-level strings.TrimFunc
// pinned by type information (a shadowed strings or a same-named method
// never matches — and TrimLeftFunc/TrimRightFunc never match, having no
// TrimLeftSpace/TrimRightSpace counterpart), and the second argument is
// (possibly parenthesized) exactly the standard library's package-level
// unicode.IsSpace — a *types.Func from package unicode with a nil
// receiver, which rejects wrapper funcs, func-typed variables, a
// shadowed unicode, and same-named methods. It returns the callee
// selector (whose Sel the fix renames).
func ps5034Match(pass *analysis.Pass, call *ast.CallExpr) (sel *ast.SelectorExpr, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	trimObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || trimObj.Pkg() == nil || trimObj.Pkg().Path() != "strings" || trimObj.Name() != "TrimFunc" {
		return nil, false
	}
	if sig, isSig := trimObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	psel, isPSel := ps2108Unparen(call.Args[1]).(*ast.SelectorExpr)
	if !isPSel {
		return nil, false
	}
	predObj, isFunc := pass.TypesInfo.Uses[psel.Sel].(*types.Func)
	if !isFunc || predObj.Pkg() == nil || predObj.Pkg().Path() != "unicode" || predObj.Name() != "IsSpace" {
		return nil, false
	}
	if sig, isSig := predObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	return sel, true
}

// ps5034Fix builds the strings.TrimSpace(s) rewrite for one site, or nil
// when a guard fails and the report must stay advisory. Two edits: the
// callee's Sel identifier "TrimFunc" becomes "TrimSpace" (the strings
// qualifier stays verbatim, so an aliased import is reused as-is), and
// the span from the kept source argument's end through the predicate and
// the original closing parenthesis is replaced by ")". The source
// argument stays untouched in place, preserving its text and single
// evaluation. The import edit, when needed, is appended later, once per
// file.
func ps5034Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	src := call.Args[0]
	// The replaced span runs from the source argument's end through the
	// closing parenthesis; a comment there would be silently destroyed —
	// advisory then. (The other edit replaces a bare identifier, which
	// cannot hold comments; a comment BEFORE the source argument sits in
	// an untouched span and survives.)
	if ps2111CommentIn(f, src.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with TrimSpace(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("TrimSpace")},
			{Pos: src.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}
