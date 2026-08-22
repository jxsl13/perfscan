package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2018 reports string(bytes.Repeat([]byte(s), n)) with s a plain
// string — a round-trip through []byte that allocates three times and
// touches the full-length result twice — and rewrites it to
// strings.Repeat(s, n), which builds the identical repetition in a
// single allocation. This is the string-result mirror of PS2011 (whose
// []byte-result direction stays advisory for capacity reasons that do
// not exist here) and a member of the PS2012/PS2016/PS2017 roundtrip
// family. The fix requires a provably non-negative constant count (the
// same gate PS2003 applies to Repeat hoists); other counts stay
// advisory.
var PS2018 = register(&lint.Check{
	ID:       "PS2018",
	Category: "alloc",
	Slug:     "string-bytes-repeat-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.Repeat([]byte(s), n)) allocates three times; strings.Repeat(s, n) builds the identical repetition once",
		Text: `string(bytes.Repeat([]byte(s), n)) pays THREE allocations:
[]byte(s) copies the seed, bytes.Repeat allocates and fills a
len(s)*n buffer, and the outer string(...) copies that whole buffer
into a fresh immutable string — the full-length result is written
twice. strings.Repeat(s, n) fills a single len(s)*n buffer and
returns it as the string zero-copy: one allocation, every byte
written once. Two allocations and one full-length copy are
eliminated, and the saving scales with the output length. This is the
exact mirror of PS2011's measured win, in the direction where the
rewrite IS mechanical: the result here is a string, which has no
observable capacity, so the capacity divergence that keeps PS2011's
[]byte-result direction advisory cannot exist.

The value is bit-identical for every input. Both functions perform
pure byte-level repetition — the bytes of the seed laid down count
times, no case folding, no normalization, no UTF-8 validation — so
for every seed (empty, multi-byte UTF-8, invalid UTF-8) and every
count the produced contents are byte-for-byte the same, pinned by the
runtime differential suite including the seeds strings.Repeat
special-cases via lookup tables (' ', '-', '0', '=', '\t') and
outputs long enough to cross its 8KB chunking strategy. s and n are
each evaluated exactly once in both forms, so side effects keep their
count and order.

The panic paths are why the fix is gated on the count. bytes.Repeat
and strings.Repeat panic under IDENTICAL conditions — count < 0, or
len(s)*count overflowing int, computed from the same lengths against
the same limit — but with different message prefixes ("bytes:
negative Repeat count" vs "strings: ..."). The fix therefore requires
the count to be a provably non-negative integer constant (the same
gate PS2003 applies before hoisting strings.Repeat, and the gate
PS2011's doc names): that removes the negative-count panic — the only
panic path reachable with realistic data — entirely. The overflow
path remains, but both forms panic on exactly the same inputs there
(only the recovered message's package prefix differs, the same
both-panic residual PS3111 accepts for slices.MaxFunc -> slices.Max),
and reaching it with a plausible constant count requires a seed of
petabytes — with count 64 the seed must exceed 2^57 bytes on 64-bit.
A non-constant or negative-constant count keeps the report advisory:
a runtime negative count would panic with a different recoverable
message after the rewrite.

One honest nuance that is NOT observable in value semantics: for
count 1, strings.Repeat returns s itself (sharing its backing memory)
where the original allocated two copies. Strings are immutable, so no
Go program can distinguish the values.

The automatic fix applies only when type information proves the
shape: the outer conversion target is the predeclared string itself
(a named string type or a shadowed identifier does not match), the
callee is the standard library's package-level bytes.Repeat (a
shadowed bytes or a same-named method never matches), the inner
conversion target is exactly the predeclared []byte (a defined slice
type is not matched), and the seed's static type is a plain string —
a NAMED string operand is excluded because strings.Repeat(s, n) would
not compile for it. The count needs no type constraint of its own
beyond the constant gate: both functions take the identical int
parameter, so whatever compiled for bytes.Repeat compiles unchanged
for strings.Repeat. The seed and count expressions are kept
byte-verbatim in place; only the punctuation around them is replaced,
so the replacement is a call wherever the original call was legal and
never needs parentheses. The fix edits imports as needed: strings is
added when missing (reusing the file's existing alias when it imports
strings under another name), and the bytes import is dropped when the
rewrites remove the file's last bytes reference. A comment inside the
replaced punctuation, a shadowed or dot/blank strings import at the
call site, or a cgo file (whose import block must never be edited)
keeps that report advisory.`,
		Before: `out := string(bytes.Repeat([]byte(s), 64))`,
		After:  `out := strings.Repeat(s, 64)`,
		MeasuredWin: `BenchmarkPS2018 (seed "ab", count 64, 128-byte output,
Apple M2 Pro, go1.26): string(bytes.Repeat([]byte(s), 64)) 82.7 ns/op,
256 B/op, 2 allocs/op -> strings.Repeat(s, 64) 65.2 ns/op, 128 B/op,
1 alloc/op (~1.3x faster, half the memory — the eliminated allocation
and copy grow with the output length; the []byte(s) seed copy is
stack-allocated here and costs a third allocation in shapes escape
analysis cannot prove).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2018",
		Doc:  "string(bytes.Repeat([]byte(s), n)) allocates the seed copy, the repeat buffer, and a string copy of it; strings.Repeat(s, n) builds the identical repetition in a single allocation",
		Run:  runPS2018,
	},
})

const ps2018Msg = "string(bytes.Repeat([]byte(s), n)) copies s into a throwaway []byte, fills a repeat buffer, and copies that whole buffer into a new string; strings.Repeat(s, n) builds the identical repetition in a single allocation — same bytes, two allocations and a full-length copy fewer"

func runPS2018(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2012/PS2016).
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
			s, count, matched := ps2018Match(pass, conv)
			if !matched {
				return true
			}
			var fix *analysis.SuggestedFix
			if ps2018NonNegConstCount(pass, count) {
				fix = ps2018Fix(pass, f, conv, s, count)
			}
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim seed/count spans, whose edits never overlap this
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
		// Each fixable site holds exactly one bytes reference (the
		// qualifier of bytes.Repeat); when those are all of the file's
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
			// PS2012/PS2016).
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
				Message: ps2018Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2018Match matches conv against string(bytes.Repeat([]byte(s), n))
// with s statically a plain string: the outer conversion target is the
// predeclared string itself, the callee is the standard library's
// package-level bytes.Repeat pinned by type information, the inner
// conversion target is exactly the predeclared []byte, and the seed's
// static type is a plain (possibly untyped-constant) string. It returns
// the seed s and the count expression.
func ps2018Match(pass *analysis.Pass, conv *ast.CallExpr) (s, count ast.Expr, ok bool) {
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
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "Repeat" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil, nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, nil, false
	}
	inner, isConv := ps2108Unparen(call.Args[0]).(*ast.CallExpr)
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
		// A NAMED string operand is deliberately not matched:
		// strings.Repeat(s, n) would not compile for it, and the
		// semantics belong to the named type.
		return nil, nil, false
	}
	return arg, call.Args[1], true
}

// ps2018NonNegConstCount reports whether count is a provably
// non-negative integer constant — the gate (shared with PS2003's Repeat
// hoist) that removes the negative-count panic path, the only panic
// path whose divergent message prefix is reachable with realistic
// data. A constant expression additionally has no side effects, so
// keeping it byte-verbatim is trivially evaluation-preserving.
func ps2018NonNegConstCount(pass *analysis.Pass, count ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[count]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return false
	}
	return constant.Sign(tv.Value) >= 0
}

// ps2018Fix builds the strings.Repeat(s, n) rewrite for one site, or
// nil when a guard fails and the report must stay advisory. Only the
// punctuation around the seed and the count is replaced; both
// expressions stay untouched in place, preserving their text and single
// evaluation (same technique as PS2012/PS2016). Import edits are
// appended later, once per file.
func ps2018Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s, count ast.Expr) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the two kept
	// expressions; a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) ||
		ps2111CommentIn(f, s.End(), count.Pos()) ||
		ps2111CommentIn(f, count.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + ".Repeat(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + ".Repeat(")},
			{Pos: s.End(), End: count.Pos(), NewText: []byte(", ")},
			{Pos: count.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}
