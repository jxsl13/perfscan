package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2017 reports string(bytes.Map(f, []byte(s))) with s a plain string —
// a per-rune mapping round-trip through []byte that pays two throwaway
// copies on top of the one allocation the mapping itself needs. It
// rewrites to strings.Map(f, s), which performs the identical mapping
// reading straight off the string. The structurally identical
// case-mapping and Trim-family round-trips are PS5010/PS5011 and
// PS2012/PS5005/PS5006.
var PS2017 = register(&lint.Check{
	ID:       "PS2017",
	Category: "alloc",
	Slug:     "string-bytes-map-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.Map(f, []byte(s))) pays two throwaway copies; strings.Map(f, s) is the same per-rune mapping with at most the result allocation",
		Text: `string(bytes.Map(f, []byte(s))) does three allocation-worthy
steps: []byte(s) heap-copies s into a fresh byte slice, bytes.Map
allocates and fills a new result slice, and string(...) heap-copies
that result into yet another string. strings.Map(f, s) performs the
identical per-rune mapping reading directly off the string header and
allocates only the result — and when f leaves every rune unchanged it
returns s itself, allocating NOTHING. The mapping work is the same in
both spellings; the entire saving is data movement: up to two extra
allocations and two full copies per call drop away.

The rewrite is bit-identical for every input. []byte(s) is a byte-exact
copy of s (invalid UTF-8 included), and the two Map implementations
share the same contract executed the same way: decode s as UTF-8 rune
by rune (utf8.DecodeRune in bytes vs the range-over-string decoder plus
DecodeRuneInString in strings, which agree on every byte sequence — an
invalid byte yields RuneError with width 1 in both), call the SAME
mapping function f exactly once per rune position, in order, with the
same rune arguments (so any side effects of f are preserved verbatim,
panics included), drop runes for which f returns a negative value, and
UTF-8-encode every non-negative result via the same utf8 machinery
(out-of-range or surrogate results encode as RuneError in both). The
subtle corners coincide too: an invalid input byte whose RuneError maps
to itself is re-encoded as the three replacement-character bytes by
BOTH spellings, a literal U+FFFD in the input survives byte-for-byte in
both, and the empty input maps to "" on both sides (string of an empty
[]byte is ""). f and s are kept byte-verbatim in place, evaluated
exactly once each and in the same order as before. The runtime
differential test pins the equivalence over exhaustive short inputs on
an adversarial alphabet, targeted Unicode edge-case seeds, randomized
inputs biased toward invalid UTF-8, and a spectrum of mapping functions
(identity, case tables, rune-dropping, rune-growing, RuneError-pinning,
out-of-range results, and a call-order recorder).

There is no memory-retention nuance: strings.Map either returns a
freshly built string or (when f changes nothing) s itself — it never
pins a larger backing array than the original spelling did.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is the
standard library's package-level bytes.Map (a shadowed bytes or a
same-named method never matches), the second argument is a conversion
to exactly the predeclared []byte (a defined slice type is not
matched), and that conversion's operand is statically a plain string —
a NAMED string operand is excluded because the strings twin would not
compile for it. The mapping argument f needs no restriction: bytes.Map
and strings.Map declare the identical parameter type func(rune) rune,
so any expression legal in one call is legal verbatim in the other.
Both arguments are kept byte-verbatim in place; only the punctuation
around them is replaced, so the replacement is a call wherever the
original call was legal and never needs parentheses. The fix edits
imports as needed: strings is added when missing (reusing the file's
existing alias when it imports strings under another name), and the
bytes import is dropped when the rewrites remove the file's last bytes
reference. A comment inside the replaced punctuation, an inner
conversion spelled through a type alias rather than the literal []byte
(deleting the alias reference could orphan the alias's import), a
shadowed or dot/blank strings import at the call site, or a cgo file
(whose import block must never be edited) keeps that report advisory.`,
		Before: `out := string(bytes.Map(f, []byte(s)))`,
		After:  `out := strings.Map(f, s)`,
		MeasuredWin: `BenchmarkPS2017 (Apple M2 Pro, go1.26): mapping
unicode.ToUpper over a 55-byte mixed-case input containing ß (the
mapping must allocate), string(bytes.Map(f, []byte(s))) 200.7 ns/op,
128 B/op, 2 allocs/op -> strings.Map(f, s) 172.5 ns/op, 64 B/op,
1 alloc/op (~1.2x, half the allocations). When f changes nothing the
win is larger: 186.1 ns/op, 128 B/op, 2 allocs/op -> 76.5 ns/op,
0 B/op, 0 allocs/op (~2.4x, allocation-free — strings.Map's no-change
path returns s itself). The copy savings grow linearly with len(s).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2017",
		Doc:  "string(bytes.Map(f, []byte(s))) round-trips s through two throwaway copies; strings.Map(f, s) performs the identical per-rune mapping reading straight off the string",
		Run:  runPS2017,
	},
})

const ps2017Msg = "string(bytes.Map(f, []byte(s))) copies s into a throwaway []byte and copies the mapped result into a new string; strings.Map(f, s) performs the identical per-rune mapping reading straight off the string — same bytes, same f calls, up to two allocations fewer"

func runPS2017(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2012/PS5010).
		type site struct {
			conv *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			conv, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			mapFn, s, matched := ps2017Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps2017Fix(pass, f, conv, mapFn, s)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim f or s spans, whose edits never overlap this
			// site's punctuation edits.
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
		// Each fixable site removes exactly one bytes reference (the
		// qualifier of bytes.Map — any bytes reference inside the
		// verbatim f or s stays); when those are all of the file's bytes
		// references, the rewrites orphan the import and the fix must
		// drop it (the runner never prunes imports itself).
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
			// PS2012/PS5010).
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
				Message: ps2017Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2017Match matches conv against string(bytes.Map(f, []byte(s))) with
// s statically a plain string: the outer conversion target is the
// predeclared string itself, the callee is the standard library's
// package-level bytes.Map pinned by type information, the second
// argument is a conversion to exactly the predeclared []byte, and that
// conversion's operand has a plain (possibly untyped-constant) string
// type. It returns the mapping expression f and the operand s. f itself
// needs no shape restriction: bytes.Map and strings.Map declare the
// identical parameter type func(rune) rune, so any f that compiles in
// the Before-shape compiles verbatim in the After-shape.
func ps2017Match(pass *analysis.Pass, conv *ast.CallExpr) (mapFn, s ast.Expr, ok bool) {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, nil, false
	}
	if !ps2108IsUniverseString(pass, conv.Fun) {
		return nil, nil, false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, nil, false
	}
	fnObj, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fnObj.Pkg() == nil || fnObj.Pkg().Path() != "bytes" || fnObj.Name() != "Map" {
		return nil, nil, false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, false
	}
	inner, isConv := ps2108Unparen(call.Args[1]).(*ast.CallExpr)
	if !isConv || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, nil, false
	}
	if !ps2108IsByteSliceConv(pass, inner.Fun) {
		return nil, nil, false
	}
	arg := inner.Args[0]
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Type == nil {
		return nil, nil, false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		// A NAMED string operand is deliberately not matched: the
		// strings twin would not compile for it, and the semantics
		// belong to the named type.
		return nil, nil, false
	}
	return call.Args[0], arg, true
}

// ps2017Fix builds the strings.Map(f, s) rewrite for one site, or nil
// when a guard fails and the report must stay advisory. Only the
// punctuation around the two kept arguments is replaced; f and s stay
// untouched in place, preserving their text, their single evaluation,
// and their evaluation order (same technique as PS2012/PS5010). Import
// edits are appended later, once per file.
func ps2017Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, mapFn, s ast.Expr) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The rewrite deletes the inner conversion's type expression. When
	// that type is spelled through an alias identifier (worse, a
	// qualified one from another package), deleting it could orphan an
	// import the runner would never prune — so the fix only fires on the
	// literal []byte spelling and leaves alias spellings advisory.
	if !ps2017IsLiteralByteSlice(pass, conv) {
		return nil
	}
	// The replaced spans are the punctuation around the kept arguments;
	// a comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, conv.Pos(), mapFn.Pos()) ||
		ps2111CommentIn(f, mapFn.End(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + ".Map(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: mapFn.Pos(), NewText: []byte(stringsName + ".Map(")},
			{Pos: mapFn.End(), End: s.Pos(), NewText: []byte(", ")},
			{Pos: s.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}

// ps2017IsLiteralByteSlice reports whether conv's inner []byte
// conversion (already matched by ps2017Match) is spelled as the literal
// type `[]byte` — an ArrayType with no length whose element is the
// universe byte identifier — as opposed to an alias identifier whose
// deletion could orphan an import.
func ps2017IsLiteralByteSlice(pass *analysis.Pass, conv *ast.CallExpr) bool {
	call := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	inner := ps2108Unparen(call.Args[1]).(*ast.CallExpr)
	at, isArr := ps2108Unparen(inner.Fun).(*ast.ArrayType)
	if !isArr || at.Len != nil {
		return false
	}
	elt, isIdent := at.Elt.(*ast.Ident)
	return isIdent && pass.TypesInfo.Uses[elt] == types.Universe.Lookup("byte")
}
