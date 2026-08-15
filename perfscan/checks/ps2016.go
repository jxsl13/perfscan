package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2016 reports string(bytes.TrimFunc([]byte(s), f)) — and the
// TrimLeftFunc/TrimRightFunc siblings — with s a plain string: a
// round-trip through []byte that pays two throwaway copies. It rewrites
// to strings.TrimFunc(s, f), which performs the identical trim and
// returns a zero-copy substring of s. This is the rune-predicate twin of
// PS5005 (the cutset Trim family); the predicate-free TrimSpace shape is
// PS2012.
var PS2016 = register(&lint.Check{
	ID:       "PS2016",
	Category: "alloc",
	Slug:     "string-bytes-trimfunc-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.TrimFunc([]byte(s), f)) pays two throwaway copies; strings.TrimFunc(s, f) is the same trim with zero copies",
		Text: `string(bytes.TrimFunc([]byte(s), f)) heap-copies s into a
fresh []byte, trims it, and then heap-copies the trimmed subslice back
into a new string. strings.TrimFunc(s, f) performs the identical trim
directly on the string and returns a SUBSTRING of s — a slice of the
original string header, zero allocations and zero copies. The trim
itself is the same work in both spellings; the entire saving is data
movement: two allocations and two full copies per call drop to none.
The same holds for the TrimLeftFunc and TrimRightFunc siblings. This is
the rune-predicate twin of PS5005 (cutset Trim/TrimLeft/TrimRight) and
of PS2012 (TrimSpace).

The rewrite is bit-identical for every input, and it preserves f's
observable behavior exactly. bytes.TrimFunc and strings.TrimFunc share
one rune-iteration core: TrimFunc is TrimRightFunc(TrimLeftFunc(s, f),
f) in both packages, TrimLeftFunc scans forward with indexFunc and
TrimRightFunc scans backward with lastIndexFunc, and the two packages'
decoders agree on every byte sequence — utf8.DecodeRune vs
DecodeRuneInString and DecodeLastRune vs DecodeLastRuneInString all
yield RuneError with width 1 on any invalid sequence. So f — the SAME
func value, its parameter type being the identical func(rune) bool in
both signatures — is called on the SAME runes in the SAME order and
count, side effects and panics included (a nil f panics at the same
point in both forms). The trim boundaries therefore coincide, the
surviving span is byte-for-byte identical, and string(subslice) equals
the substring for every s, including the empty string and invalid
UTF-8. bytes' historical nil result on a fully-trimmed TrimLeftFunc
input is invisible here: string(nil) == "". Each argument is evaluated
exactly once, left to right, in both forms. The runtime differential
test pins both the value identity and the f call sequence over
exhaustive short inputs crossed with adversarial predicates, targeted
seeds, and randomized long inputs.

One honest nuance that is NOT observable in value semantics: the
rewritten form returns a substring sharing s's backing memory, where
the original allocated fresh memory. Strings are immutable, so no Go
program can distinguish the two values — but the substring keeps s's
backing array reachable for as long as the result lives. In the rare
case where a tiny trimmed result must outlive a huge s, follow up with
strings.Clone; that is an explicit memory-retention decision, not part
of this rewrite.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is the
standard library's package-level bytes.TrimFunc, bytes.TrimLeftFunc, or
bytes.TrimRightFunc (a shadowed bytes or a same-named method never
matches), the inner conversion target is exactly the predeclared []byte
(a defined slice type is not matched), and the operand's static type is
a plain string — a NAMED string operand is excluded because the
strings twin would not compile for it. The predicate argument needs no
constraint of its own: its parameter type is the identical func(rune)
bool in both spellings, so whatever compiled for bytes.TrimFunc
compiles unchanged for strings.TrimFunc. The operand and predicate
expressions are kept byte-verbatim in place; only the punctuation
around them is replaced, so the replacement is a call wherever the
original call was legal and never needs parentheses. The fix edits
imports as needed: strings is added when missing (reusing the file's
existing alias when it imports strings under another name), and the
bytes import is dropped when the rewrites remove the file's last bytes
reference. A comment inside the replaced punctuation, a shadowed or
dot/blank strings import at the call site, or a cgo file (whose import
block must never be edited) keeps that report advisory. IndexFunc and
LastIndexFunc return offsets into different values (byte slice vs
string) — offsets happen to agree, but they are a different shape and
deliberately not matched here.`,
		Before: `out := string(bytes.TrimFunc([]byte(s), unicode.IsSpace))`,
		After:  `out := strings.TrimFunc(s, unicode.IsSpace)`,
		MeasuredWin: `BenchmarkPS2016 (a 54-byte line trimmed with
unicode.IsSpace edges, Apple M2 Pro, go1.26):
string(bytes.TrimFunc([]byte(s), f)) 40.6 ns/op, 48 B/op, 1 alloc/op ->
strings.TrimFunc(s, f) 23.7 ns/op, 0 B/op, 0 allocs/op (~1.7x faster,
allocation-free). On current gc, escape analysis already elides the
[]byte(s) copy in this exact shape — the measured alloc is the
string(...) result copy; in shapes escape analysis cannot prove (and on
toolchains without the optimization) the Before pays both copies. The
saving grows with len(s), since the copies are full-length while the
After is O(trimmed edges) plus no copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2016",
		Doc:  "string(bytes.TrimFunc([]byte(s), f)) round-trips s through two throwaway copies; strings.TrimFunc(s, f) calls the same predicate on the same runes and returns the identical trimmed result as a zero-copy substring",
		Run:  runPS2016,
	},
})

func ps2016Msg(fn string) string {
	return "string(bytes." + fn + "([]byte(s), f)) copies s into a throwaway []byte and copies the trimmed span back into a new string; strings." + fn + "(s, f) calls the same predicate on the same runes and returns a zero-copy substring — same bytes, two allocations fewer"
}

func runPS2016(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2012/PS5005).
		type site struct {
			conv *ast.CallExpr
			fn   string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			conv, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			s, pred, fn, matched := ps2016Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps2016Fix(pass, f, conv, s, pred, fn)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand/predicate spans, whose edits never overlap
			// this site's punctuation edits.
			return true
		})
		if len(sites) == 0 {
			continue
		}
		stringsImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"strings"` {
				stringsImported = true
				break
			}
		}
		needStrings := fixable > 0 && !stringsImported
		// Each fixable site holds exactly one bytes reference (the
		// qualifier of bytes.Trim*Func); when those are all of the file's
		// bytes references, the rewrites orphan the import and the fix
		// must drop it (the runner never prunes imports itself).
		orphansBytes := fixable > 0 && pkgRefCount(pass, f, "bytes") == fixable
		importEdits, importsOK := ps2012ImportEdits(f, needStrings, orphansBytes)
		if !importsOK {
			// cgo file needing import surgery, or a bytes spec we could
			// not locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS2012/PS5005).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.conv.Pos(),
				End:     st.conv.End(),
				Message: ps2016Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2016Match matches conv against
// string(bytes.TrimFunc([]byte(s), f)) — or the TrimLeftFunc/
// TrimRightFunc siblings — with s statically a plain string: the outer
// conversion target is the predeclared string itself, the callee is the
// standard library's package-level bytes.TrimFunc/TrimLeftFunc/
// TrimRightFunc pinned by type information, the inner conversion target
// is exactly the predeclared []byte, and the operand's static type is a
// plain (possibly untyped-constant) string. The predicate argument needs
// no constraint of its own: its parameter type is the identical
// func(rune) bool in both spellings, so whatever compiled for
// bytes.TrimFunc compiles unchanged for strings.TrimFunc. It returns
// the operand s, the predicate expression, and the function name.
func ps2016Match(pass *analysis.Pass, conv *ast.CallExpr) (s, pred ast.Expr, fn string, ok bool) {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	if !ps2108IsUniverseString(pass, conv.Fun) {
		return nil, nil, "", false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, nil, "", false
	}
	fnObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fnObj.Pkg() == nil || fnObj.Pkg().Path() != "bytes" {
		return nil, nil, "", false
	}
	switch fnObj.Name() {
	case "TrimFunc", "TrimLeftFunc", "TrimRightFunc":
	default:
		// The cutset-taking Trim family is PS5005; IndexFunc and
		// LastIndexFunc return offsets, a different shape — out of scope.
		return nil, nil, "", false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, "", false
	}
	inner, isConv := ps2108Unparen(call.Args[0]).(*ast.CallExpr)
	if !isConv || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, nil, "", false
	}
	if !ps2108IsByteSliceConv(pass, inner.Fun) {
		return nil, nil, "", false
	}
	arg := inner.Args[0]
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Type == nil {
		return nil, nil, "", false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		// A NAMED string operand is deliberately not matched: the
		// strings twin would not compile for it, and the semantics
		// belong to the named type.
		return nil, nil, "", false
	}
	return arg, call.Args[1], fnObj.Name(), true
}

// ps2016Fix builds the strings.Trim*Func(s, f) rewrite for one site, or
// nil when a guard fails and the report must stay advisory. Only the
// punctuation around the operand and the predicate is replaced; both
// expressions stay untouched in place, preserving their text and single
// evaluation (same technique as PS2012/PS5005). Import edits are
// appended later, once per file.
func ps2016Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s, pred ast.Expr, fn string) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the two kept
	// expressions; a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), pred.Pos()) ||
		ps2111CommentIn(f, pred.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + "." + fn + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + "." + fn + "(")},
			{Pos: s.End(), End: pred.Pos(), NewText: []byte(", ")},
			{Pos: pred.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}
