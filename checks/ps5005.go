package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5005 reports string(bytes.Trim([]byte(s), cutset)) — and the
// TrimLeft/TrimRight siblings — with s a plain string: a round-trip
// through []byte that pays two throwaway copies. It rewrites to
// strings.Trim(s, cutset), which performs the identical trim and returns
// a zero-copy substring of s. The cutset-free TrimSpace shape is PS2012.
var PS5005 = register(&lint.Check{
	ID:       "PS5005",
	Category: "alloc",
	Slug:     "string-bytes-trim-cutset-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.Trim([]byte(s), cutset)) pays two throwaway copies; strings.Trim(s, cutset) is the same trim with zero copies",
		Text: `string(bytes.Trim([]byte(s), cutset)) heap-copies s into a
fresh []byte, trims it, and then heap-copies the trimmed subslice back
into a new string. strings.Trim(s, cutset) performs the identical trim
directly on the string and returns a SUBSTRING of s — a slice of the
original string header, zero allocations and zero copies. The trim
itself is the same work in both spellings; the entire saving is data
movement: two allocations and two full copies per call drop to none.
The same holds for the TrimLeft and TrimRight siblings. This is the
cutset-taking twin of PS2012 (TrimSpace).

The rewrite is bit-identical for every input. bytes.Trim/TrimLeft/
TrimRight and strings.Trim/TrimLeft/TrimRight share the same trimming
machinery: the single-ASCII-byte fast path (len(cutset) == 1 &&
cutset[0] < RuneSelf), the makeASCIISet/asciiSet.contains path for
all-ASCII cutsets, and the rune path decoding with utf8.DecodeRune
(bytes) vs DecodeRuneInString (strings), which agree on every byte
sequence — an invalid sequence yields RuneError with width 1 in both.
The rune-membership tests (bytes' range-based containsRune vs
strings.ContainsRune/IndexRune) agree on every cutset too, including
cutsets holding invalid UTF-8: both treat a decoded RuneError as
matching any invalid sequence in the cutset, ASCII lookups can never
land inside a multi-byte encoding (those bytes are all >= 0x80), and
UTF-8's self-synchronization makes the substring search and the
sequential decode find the same multi-byte runes. So the trim
boundaries coincide, the surviving span is byte-for-byte identical, and
string(subslice) equals the substring. bytes' historical nil result on
a fully-trimmed input is invisible here: string(nil) == "". The cutset
argument has parameter type string in BOTH spellings, so it is passed
through untouched — evaluated exactly once, same value, no new type
constraint — and s is likewise evaluated exactly once in both forms.
The runtime differential test pins the equivalence for all three
functions over exhaustive short inputs crossed with adversarial cutsets
(single-byte, ASCII-set, multi-byte-rune, and invalid-UTF-8 cutsets),
targeted seeds, and randomized long inputs.

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
standard library's package-level bytes.Trim, bytes.TrimLeft, or
bytes.TrimRight (a shadowed bytes or a same-named method never
matches), the inner conversion target is exactly the predeclared []byte
(a defined slice type is not matched), and the operand's static type is
a plain string — a NAMED string operand is excluded because the
strings twin would not compile for it. The operand and cutset
expressions are kept byte-verbatim in place; only the punctuation
around them is replaced, so the replacement is a call wherever the
original call was legal and never needs parentheses. The fix edits
imports as needed: strings is added when missing (reusing the file's
existing alias when it imports strings under another name), and the
bytes import is dropped when the rewrites remove the file's last bytes
reference. A comment inside the replaced punctuation, a shadowed or
dot/blank strings import at the call site, or a cgo file (whose import
block must never be edited) keeps that report advisory. TrimPrefix and
TrimSuffix are a different shape (their second argument is a []byte
too) and are deliberately not matched.`,
		Before: `out := string(bytes.Trim([]byte(s), cutset))`,
		After:  `out := strings.Trim(s, cutset)`,
		MeasuredWin: `BenchmarkPS5005 (a 54-byte line trimmed with the
cutset " \t\r\n-", Apple M2 Pro, go1.26): string(bytes.Trim([]byte(s),
cutset)) 32.9 ns/op, 48 B/op, 1 alloc/op -> strings.Trim(s, cutset)
11.3 ns/op, 0 B/op, 0 allocs/op (~2.9x faster, allocation-free). On
current gc, escape analysis already elides the []byte(s) copy in this
exact shape — the measured alloc is the string(...) result copy; in
shapes escape analysis cannot prove (and on toolchains without the
optimization) the Before pays both copies. The saving grows with
len(s), since the copies are full-length while the After is O(trimmed
edges) plus no copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5005",
		Doc:  "string(bytes.Trim([]byte(s), cutset)) round-trips s through two throwaway copies; strings.Trim(s, cutset) returns the identical trimmed result as a zero-copy substring",
		Run:  runPS5005,
	},
})

func ps5005Msg(fn string) string {
	return "string(bytes." + fn + "([]byte(s), cutset)) copies s into a throwaway []byte and copies the trimmed span back into a new string; strings." + fn + "(s, cutset) performs the identical trim and returns a zero-copy substring — same bytes, two allocations fewer"
}

func runPS5005(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2012).
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
			s, cutset, fn, matched := ps5005Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps5005Fix(pass, f, conv, s, cutset, fn)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand/cutset spans, whose edits never overlap
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
		// qualifier of bytes.Trim*); when those are all of the file's
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
			// PS2012/PS2110).
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
				Message: ps5005Msg(st.fn),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5005Match matches conv against
// string(bytes.Trim([]byte(s), cutset)) — or the TrimLeft/TrimRight
// siblings — with s statically a plain string: the outer conversion
// target is the predeclared string itself, the callee is the standard
// library's package-level bytes.Trim/TrimLeft/TrimRight pinned by type
// information, the inner conversion target is exactly the predeclared
// []byte, and the operand's static type is a plain (possibly
// untyped-constant) string. The cutset argument needs no constraint of
// its own: its parameter type is string in both spellings, so whatever
// compiled for bytes.Trim compiles unchanged for strings.Trim. It
// returns the operand s, the cutset expression, and the function name.
func ps5005Match(pass *analysis.Pass, conv *ast.CallExpr) (s, cutset ast.Expr, fn string, ok bool) {
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
	case "Trim", "TrimLeft", "TrimRight":
	default:
		// TrimPrefix/TrimSuffix take a []byte second argument — a
		// different shape, deliberately out of scope.
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

// ps5005Fix builds the strings.Trim*(s, cutset) rewrite for one site, or
// nil when a guard fails and the report must stay advisory. Only the
// punctuation around the operand and the cutset is replaced; both
// expressions stay untouched in place, preserving their text and single
// evaluation (same technique as PS2012). Import edits are appended
// later, once per file.
func ps5005Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s, cutset ast.Expr, fn string) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the two kept
	// expressions; a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), cutset.Pos()) ||
		ps2111CommentIn(f, cutset.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + "." + fn + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + "." + fn + "(")},
			{Pos: s.End(), End: cutset.Pos(), NewText: []byte(", ")},
			{Pos: cutset.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}
