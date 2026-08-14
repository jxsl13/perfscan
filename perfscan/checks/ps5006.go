package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5006 reports string(bytes.TrimPrefix([]byte(s), []byte(p))) — and
// the TrimSuffix sibling — with s and p plain strings: a round-trip
// through []byte that pays throwaway conversions. It rewrites to
// strings.TrimPrefix(s, p), which performs the identical byte-wise trim
// and returns a zero-copy substring of s. The cutset-taking
// Trim/TrimLeft/TrimRight shape is PS5005; the cutset-free TrimSpace
// shape is PS2012.
var PS5006 = register(&lint.Check{
	ID:       "PS5006",
	Category: "alloc",
	Slug:     "string-bytes-trimprefix-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.TrimPrefix([]byte(s), []byte(p))) pays throwaway copies; strings.TrimPrefix(s, p) is the same trim with zero copies",
		Text: `string(bytes.TrimPrefix([]byte(s), []byte(p))) converts s and p
into throwaway byte slices, trims, and then heap-copies the surviving
subslice back into a new string. strings.TrimPrefix(s, p) performs the
identical trim directly on the string and returns a SUBSTRING of s — a
slice of the original string header, zero allocations and zero copies.
Escape analysis on current gc already stack-allocates the two []byte
views (bytes.TrimPrefix only reslices its inputs), but the final
string(...) conversion MUST heap-copy the trimmed span into a fresh
immutable string — that allocation is real and unavoidable in the bytes
spelling. The same holds for the TrimSuffix sibling. This is the
prefix/suffix twin of PS5005 (cutset trims) and PS2012 (TrimSpace).

The rewrite is bit-identical for every input. bytes.TrimPrefix and
strings.TrimPrefix share identical semantics: a pure byte-wise prefix
comparison (HasPrefix — a length check plus bytewise equality) followed
by reslicing off the matched prefix, with NO rune decoding, NO case
folding, and NO Unicode normalization anywhere. TrimSuffix is the
mirror image with HasSuffix. So for every (s, p) pair — empty s, empty
p, p == s, p longer than s, non-matching p, multi-byte UTF-8, invalid
UTF-8, NUL bytes — the surviving span is byte-for-byte the same and
string(subslice) equals the substring. When the whole input is trimmed
away, bytes returns an empty (possibly nil-backed) slice and
string(empty) == "" — invisible. Aliasing is unobservable: both
spellings yield an immutable string, so the fact that
strings.TrimPrefix returns a substring sharing s's memory (where the
bytes form allocated fresh memory) cannot be detected by any Go
program. s and p are each evaluated exactly once in both forms, in the
same order. The runtime differential test pins the equivalence for both
functions over exhaustive short input/prefix pairs, targeted seeds, and
randomized long inputs.

One honest nuance that is NOT observable in value semantics: the
rewritten form keeps s's backing memory reachable for as long as the
result lives. In the rare case where a tiny trimmed result must outlive
a huge s, follow up with strings.Clone; that is an explicit
memory-retention decision, not part of this rewrite.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is the
standard library's package-level bytes.TrimPrefix or bytes.TrimSuffix
(a shadowed bytes or a same-named method never matches), BOTH argument
positions are exactly a predeclared-[]byte conversion of an operand
whose static type is a plain string — a NAMED string operand on either
side is excluded because the strings twin would not compile for it, and
a genuine []byte prefix (a variable, or a []byte{...} literal with no
string source) is never rewritten because it has no string spelling.
The operand and prefix expressions are kept byte-verbatim in place;
only the punctuation around them is replaced, so the replacement is a
call wherever the original call was legal and never needs parentheses.
The fix edits imports as needed: strings is added when missing (reusing
the file's existing alias when it imports strings under another name),
and the bytes import is dropped when the rewrites remove the file's
last bytes reference. A comment inside the replaced punctuation, a
shadowed or dot/blank strings import at the call site, or a cgo file
(whose import block must never be edited) keeps that report advisory.`,
		Before: `out := string(bytes.TrimPrefix([]byte(s), []byte(p)))`,
		After:  `out := strings.TrimPrefix(s, p)`,
		MeasuredWin: `BenchmarkPS5006 (a 53-byte line with a 16-byte
prefix, Apple M2 Pro, go1.26):
string(bytes.TrimPrefix([]byte(s), []byte(p))) 16.6 ns/op, 48 B/op,
1 alloc/op -> strings.TrimPrefix(s, p) 2.52 ns/op, 0 B/op,
0 allocs/op (~6.6x faster, allocation-free). The win is NOT
compiler-elided: escape analysis stack-allocates the two []byte views
but the string(result) conversion must heap-copy the trimmed span
('string(~r0) escapes to heap'), so the allocation is real in every
Before shape. The saving grows with len(s), since the result copy is
O(len(s)-len(p)) while the After is O(len(p)) comparison plus no copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5006",
		Doc:  "string(bytes.TrimPrefix([]byte(s), []byte(p))) round-trips s through throwaway conversions; strings.TrimPrefix(s, p) returns the identical trimmed result as a zero-copy substring",
		Run:  runPS5006,
	},
})

func ps5006Msg(fn, arg string) string {
	return "string(bytes." + fn + "([]byte(s), []byte(" + arg + "))) copies the trimmed span into a new string; strings." + fn + "(s, " + arg + ") performs the identical byte-wise trim and returns a zero-copy substring — same bytes, allocation-free"
}

func runPS5006(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS5005/PS2012).
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
			s, p, fn, matched := ps5006Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps5006Fix(pass, f, conv, s, p, fn)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fn, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand/prefix spans, whose edits never overlap
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
		// qualifier of bytes.TrimPrefix/TrimSuffix); when those are all
		// of the file's bytes references, the rewrites orphan the import
		// and the fix must drop it (the runner never prunes imports
		// itself).
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
			// PS5005/PS2012).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			arg := "prefix"
			if st.fn == "TrimSuffix" {
				arg = "suffix"
			}
			diag := analysis.Diagnostic{
				Pos:     st.conv.Pos(),
				End:     st.conv.End(),
				Message: ps5006Msg(st.fn, arg),
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5006PlainStringOperand matches arg against a predeclared-[]byte
// conversion of an operand whose static type is a plain (possibly
// untyped-constant) string, returning the operand.
func ps5006PlainStringOperand(pass *analysis.Pass, arg ast.Expr) (ast.Expr, bool) {
	conv, isConv := ps2108Unparen(arg).(*ast.CallExpr)
	if !isConv || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	if !ps2108IsByteSliceConv(pass, conv.Fun) {
		return nil, false
	}
	operand := conv.Args[0]
	tv, found := pass.TypesInfo.Types[operand]
	if !found || tv.Type == nil {
		return nil, false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		// A NAMED string operand is deliberately not matched: the
		// strings twin would not compile for it, and the semantics
		// belong to the named type.
		return nil, false
	}
	return operand, true
}

// ps5006Match matches conv against
// string(bytes.TrimPrefix([]byte(s), []byte(p))) — or the TrimSuffix
// sibling — with s and p statically plain strings: the outer conversion
// target is the predeclared string itself, the callee is the standard
// library's package-level bytes.TrimPrefix/TrimSuffix pinned by type
// information, and BOTH argument positions are exactly a
// predeclared-[]byte conversion of a plain-string operand. It returns
// the operand s, the prefix/suffix operand p, and the function name.
func ps5006Match(pass *analysis.Pass, conv *ast.CallExpr) (s, p ast.Expr, fn string, ok bool) {
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
	case "TrimPrefix", "TrimSuffix":
	default:
		// Trim/TrimLeft/TrimRight take a string cutset — that shape is
		// PS5005; TrimSpace is PS2012.
		return nil, nil, "", false
	}
	if sig, isSig := fnObj.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, "", false
	}
	s, ok = ps5006PlainStringOperand(pass, call.Args[0])
	if !ok {
		return nil, nil, "", false
	}
	p, ok = ps5006PlainStringOperand(pass, call.Args[1])
	if !ok {
		// A genuine []byte prefix (a variable, or a []byte{...} literal
		// with no string source) has no string spelling — never
		// rewritten, never reported.
		return nil, nil, "", false
	}
	return s, p, fnObj.Name(), true
}

// ps5006Fix builds the strings.TrimPrefix/TrimSuffix(s, p) rewrite for
// one site, or nil when a guard fails and the report must stay advisory.
// Only the punctuation around the two operands is replaced; both
// expressions stay untouched in place, preserving their text and single
// evaluation (same technique as PS5005/PS2012). Import edits are
// appended later, once per file.
func ps5006Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s, p ast.Expr, fn string) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the two kept
	// expressions; a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), p.Pos()) ||
		ps2111CommentIn(f, p.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + "." + fn + "(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + "." + fn + "(")},
			{Pos: s.End(), End: p.Pos(), NewText: []byte(", ")},
			{Pos: p.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}
