package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3031 reports bytes.TrimFunc(b, unicode.IsSpace) — the generic
// rune-at-a-time trimmer driving the stdlib white-space classifier
// through a function pointer — and rewrites it to bytes.TrimSpace(b),
// which is DEFINED as exactly that trim and adds an all-ASCII fast path
// that inspects boundary bytes by table lookup. The []byte twin of
// PS5035 (strings.TrimFunc(s, unicode.IsSpace) -> strings.TrimSpace(s));
// the string(bytes.TrimFunc([]byte(s), f)) round-trip is PS2016, which
// diagnoses the ENCLOSING conversion — when both fire on one site the
// runner applies one fix and converges to strings.TrimSpace(s) on the
// next -fix run either way (via PS5035 after PS2016, or via PS2012
// after this check), with no oscillation.
var PS3031 = register(&lint.Check{
	ID:       "PS3031",
	Category: "indirect",
	Slug:     "bytes-trimfunc-isspace-to-trimspace",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.TrimFunc(b, unicode.IsSpace) decodes every boundary rune through an indirect call; bytes.TrimSpace(b) is the same trim with an ASCII byte-table fast path",
		Text: `bytes.TrimFunc(b, unicode.IsSpace) pays, for EVERY leading and
trailing rune it examines, a utf8.DecodeRune / DecodeLastRune and an
INDIRECT call through the predicate function pointer into
unicode.IsSpace — a call the compiler can neither inline nor
devirtualize, on top of IsSpace's own range-table logic for
non-Latin-1 runes. bytes.TrimSpace(b) computes the identical trim but
scans the boundaries with a 256-entry byte-table lookup (asciiSpace)
first: for the overwhelmingly common mostly-ASCII input it does
strictly less work per boundary byte — no rune decode, no indirect
call, no table walk — and it only falls back to the rune path when it
actually meets a byte >= utf8.RuneSelf.

The rewrite is bit-identical for every input — including the []byte
result's pointer, length, capacity, and nil-ness, the aliasing surface
a string result does not have. bytes.TrimSpace is DEFINED as the trim
over unicode.IsSpace, and its implementation literally delegates to
TrimFunc(s[lo:], unicode.IsSpace) (resp. TrimFunc(s[:hi+1], ...)) the
moment its ASCII scan meets a byte >= utf8.RuneSelf — the very call the
user wrote. On the fast path, asciiSpace marks exactly {'\t', '\n',
'\v', '\f', '\r', ' '}, which equals unicode.IsSpace restricted to
single bytes below utf8.RuneSelf — the other two Latin-1 spaces,
U+0085 (NEL) and U+00A0 (NBSP), are multi-byte in UTF-8, so as encoded
runes they reach the shared rune path in both spellings, and as RAW
bytes (invalid UTF-8) both spellings decode utf8.RuneError, which is
not a space. Every branch of both spellings returns a subslice of the
SAME backing array at the SAME start offset with the SAME length, so
the data pointer, len, and cap all coincide (TrimLeftFunc returns
s[i:] with cap = cap-i; TrimSpace's fast path returns s[lo:][:hi+1]
with cap = cap-lo; its non-ASCII fallbacks re-slice at the same
absolute offsets and delegate — the boundary bytes each fallback has
already skipped are single-byte ASCII spaces, which are
self-synchronizing in UTF-8, so the delegated rune decode sees the
identical boundaries). The all-space (and empty) input is bit-identical
too: TrimFunc returns nil via TrimLeftFunc's i == -1 branch, and
TrimSpace carries an explicit special case "to preserve previous
TrimLeftFunc behavior, returning nil instead of empty slice if all
spaces". b is evaluated exactly once in both forms; the discarded
predicate is a pure package-function reference with no side effects
and no evaluation of its own. The runtime differential test pins
pointer+len+cap+nil-ness+byte equality over exhaustive short inputs on
an adversarial alphabet (each ASCII white-space byte, NBSP/NEL bytes
both complete and truncated, bare continuation bytes, 0xFF), each also
re-sliced with spare capacity and at a nonzero offset into a larger
array, plus targeted Unicode white-space seeds and randomized inputs
biased toward boundary white space and invalid UTF-8.

The automatic fix applies only when type information proves the shape:
the callee is the standard library's package-level bytes.TrimFunc (a
shadowed bytes or a same-named method never matches) and the predicate
argument is exactly the standard library's package-level
unicode.IsSpace (a wrapper func, a func-typed variable holding
unicode.IsSpace, a shadowed unicode, or any other predicate is
rejected by the package and receiver checks). Only the full TrimFunc
maps: bytes.TrimLeftFunc and bytes.TrimRightFunc have no
TrimLeftSpace/TrimRightSpace counterpart and are never matched.
strings.TrimFunc is PS5035's site, never this check's. The source
argument is kept byte-verbatim in place — same text, same single
evaluation, and any type assignable to []byte (a named []byte type
included) stays legal because TrimSpace takes the identical parameter
type — and the bytes qualifier is kept verbatim too, so an aliased
bytes import is reused as-is. The fix deletes the predicate argument,
which removes one unicode reference; when the rewrites remove the
file's last unicode reference the orphaned unicode import is dropped
as well (the runner never prunes imports itself). A comment inside the
deleted span, a cgo file that would need that import surgery (its
import block is never edited), or a unicode import spelled more than
once keeps the report advisory.

One adjacency, not an overlap in effect: inside
string(bytes.TrimFunc([]byte(s), unicode.IsSpace)) this check's site
is the inner call while PS2016 diagnoses the enclosing conversion.
Their fixes overlap textually, so the runner applies one and skips the
other as a benign overlap; whichever wins, the next -fix run converges
to strings.TrimSpace(s) (through PS5035 or PS2012 respectively) with
no oscillation.`,
		Before: `out := bytes.TrimFunc(b, unicode.IsSpace)`,
		After:  `out := bytes.TrimSpace(b)`,
		MeasuredWin: `BenchmarkPS3031 (Apple M2 Pro, go1.26): a 51-byte
ASCII log line padded with " \t\r\n  " on both sides drops from
37.5 ns/op to 7.3 ns/op (~5.1x — the boundary runes are classified by
byte-table lookup instead of a decode plus an indirect IsSpace call
each). An already-trimmed ASCII line drops from 8.5 ns/op to 2.7 ns/op
(~3.2x — TrimSpace inspects one byte per end, TrimFunc still decodes
and indirect-calls at both ends). An NBSP-padded input measures at
parity (32.9 ns/op vs 35.5 ns/op, ~8% over the delegation) because
TrimSpace immediately hands off to the very TrimFunc call the user
wrote. 0 B/op and 0 allocs/op on every side — both spellings return a
subslice.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3031",
		Doc:  "bytes.TrimFunc(b, unicode.IsSpace) decodes every boundary rune and calls the predicate through a function pointer; bytes.TrimSpace(b) is the identical trim with an ASCII byte-table fast path",
		Run:  runPS3031,
	},
})

const ps3031Msg = "bytes.TrimFunc(b, unicode.IsSpace) decodes every boundary rune and calls unicode.IsSpace through a function pointer; bytes.TrimSpace(b) is the identical trim (it delegates to this very TrimFunc call past its fast path) with an ASCII byte-table fast path"

func runPS3031(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site deletes exactly one unicode reference (the qualifier of
		// unicode.IsSpace), and whether that orphans the unicode import
		// depends on ALL sites together (same per-file site collection as
		// PS5017/PS5018/PS5035).
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
			sel, matched := ps3031Match(pass, call)
			if !matched {
				return true
			}
			fix := ps3031Fix(f, call, sel)
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
				// convention as PS5017/PS5018/PS5035).
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
				Message: ps3031Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps3031Match matches call against bytes.TrimFunc(b, unicode.IsSpace):
// the callee is the standard library's package-level bytes.TrimFunc
// pinned by type information (a shadowed bytes or a same-named method
// never matches — and TrimLeftFunc/TrimRightFunc never match, having no
// TrimLeftSpace/TrimRightSpace counterpart), and the second argument is
// (possibly parenthesized) exactly the standard library's package-level
// unicode.IsSpace — a *types.Func from package unicode with a nil
// receiver, which rejects wrapper funcs, func-typed variables, a
// shadowed unicode, and same-named methods. It returns the callee
// selector (whose Sel the fix renames).
func ps3031Match(pass *analysis.Pass, call *ast.CallExpr) (sel *ast.SelectorExpr, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	trimObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || trimObj.Pkg() == nil || trimObj.Pkg().Path() != "bytes" || trimObj.Name() != "TrimFunc" {
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

// ps3031Fix builds the bytes.TrimSpace(b) rewrite for one site, or nil
// when a guard fails and the report must stay advisory. Two edits: the
// callee's Sel identifier "TrimFunc" becomes "TrimSpace" (the bytes
// qualifier stays verbatim, so an aliased import is reused as-is), and
// the span from the kept source argument's end through the predicate and
// the original closing parenthesis is replaced by ")". The source
// argument stays untouched in place, preserving its text and single
// evaluation. The import edit, when needed, is appended later, once per
// file.
func ps3031Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
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
