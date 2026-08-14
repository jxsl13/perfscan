package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2012 reports string(bytes.TrimSpace([]byte(s))) with s a plain string
// — a round-trip through []byte that pays two throwaway copies — and
// rewrites it to strings.TrimSpace(s), which returns a zero-copy
// substring of s with the identical byte content.
var PS2012 = register(&lint.Check{
	ID:       "PS2012",
	Category: "alloc",
	Slug:     "string-bytes-trimspace-roundtrip",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "string(bytes.TrimSpace([]byte(s))) pays two throwaway copies; strings.TrimSpace(s) is the same trim with zero copies",
		Text: `string(bytes.TrimSpace([]byte(s))) heap-copies s into a fresh
[]byte, scans it, and then heap-copies the trimmed subslice back into a
new string. strings.TrimSpace(s) performs the identical scan directly on
the string and returns a SUBSTRING of s — a slice of the original string
header, zero allocations and zero copies. The trim itself is the same
work in both spellings; the entire saving is data movement: two
allocations and two full copies per call drop to none.

The rewrite is bit-identical for every input. Both TrimSpace
implementations trim exactly the leading and trailing runes for which
unicode.IsSpace reports true ('\t', '\n', '\v', '\f', '\r', ' ', U+0085,
U+00A0, and the Unicode White_Space class), and both decode with the
same utf8 rule on malformed input (DecodeRune and DecodeRuneInString
return RuneError with width 1 for an invalid byte, and RuneError is not
a space), so the trimming boundaries coincide even on invalid UTF-8.
The surviving span is byte-for-byte the same, and string(...) of that
span equals the corresponding substring of s. s is evaluated exactly
once in both forms, so side effects keep their count and order. Neither
form is a constant expression, and both have static type string, so no
compile-time property changes. The runtime differential test pins this
equivalence over exhaustive short inputs (all byte combinations of the
whitespace classes, multi-byte Unicode spaces, and invalid UTF-8) plus
randomized long ones.

One honest nuance that is NOT observable in value semantics: the
rewritten form returns a substring sharing s's backing memory, where the
original allocated fresh memory. Strings are immutable, so no Go program
can distinguish the two values — but the substring keeps s's backing
array reachable for as long as the result lives. In the rare case where
a tiny trimmed result must outlive a huge s, follow up with
strings.Clone; that is an explicit memory-retention decision, not part
of this rewrite.

The automatic fix applies only when type information proves the shape:
the outer conversion target is the predeclared string itself (a named
string type or a shadowed identifier does not match), the callee is the
standard library's package-level bytes.TrimSpace (a shadowed bytes or a
same-named method never matches), the inner conversion target is exactly
the predeclared []byte (a defined slice type is not matched), and the
operand's static type is a plain string — a NAMED string operand is
excluded because strings.TrimSpace(s) would not compile for it. The
operand expression is kept byte-verbatim in place; only the punctuation
around it is replaced, so the replacement is a call wherever the
original call was legal and never needs parentheses. The fix edits
imports as needed: strings is added when missing (reusing the file's
existing alias when it imports strings under another name), and the
bytes import is dropped when the rewrites remove the file's last bytes
reference. A comment inside the replaced punctuation, a shadowed or
dot/blank strings import at the call site, or a cgo file (whose import
block must never be edited) keeps that report advisory.`,
		Before: `out := string(bytes.TrimSpace([]byte(s)))`,
		After:  `out := strings.TrimSpace(s)`,
		MeasuredWin: `BenchmarkPS2012 (a 55-byte line with leading and trailing
whitespace, Apple M2 Pro, go1.26): string(bytes.TrimSpace([]byte(s)))
30.1 ns/op, 48 B/op, 1 alloc/op -> strings.TrimSpace(s) 6.9 ns/op,
0 B/op, 0 allocs/op (~4.4x faster, allocation-free). On current gc,
escape analysis already elides the []byte(s) copy in this exact shape —
the measured alloc is the string(...) result copy; in shapes escape
analysis cannot prove (and on toolchains without the optimization) the
Before pays both copies. The saving grows with len(s), since the copies
are full-length while the After is O(trimmed edges) plus no copy.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2012",
		Doc:  "string(bytes.TrimSpace([]byte(s))) round-trips s through two throwaway copies; strings.TrimSpace(s) returns the identical trimmed result as a zero-copy substring",
		Run:  runPS2012,
	},
})

const ps2012Msg = "string(bytes.TrimSpace([]byte(s))) copies s into a throwaway []byte and copies the trimmed span back into a new string; strings.TrimSpace(s) performs the identical trim and returns a zero-copy substring — same bytes, two allocations fewer"

func runPS2012(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// site removes one bytes reference and may need the strings
		// import, and both decisions depend on ALL sites together (same
		// per-file site collection as PS2118/PS3104).
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
			s, matched := ps2012Match(pass, conv)
			if !matched {
				return true
			}
			fix := ps2012Fix(pass, f, conv, s)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{conv, fix})
			// Keep descending: a nested match can only sit inside the
			// verbatim operand span, whose edits never overlap this
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
		// qualifier of bytes.TrimSpace); when those are all of the file's
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
			// PS2110/PS3104).
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
				Message: ps2012Msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2012Match matches conv against string(bytes.TrimSpace([]byte(s)))
// with s statically a plain string: the outer conversion target is the
// predeclared string itself, the callee is the standard library's
// package-level bytes.TrimSpace pinned by type information, the inner
// conversion target is exactly the predeclared []byte, and the operand's
// static type is a plain (possibly untyped-constant) string. It returns
// the operand s.
func ps2012Match(pass *analysis.Pass, conv *ast.CallExpr) (s ast.Expr, ok bool) {
	if len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	if !ps2108IsUniverseString(pass, conv.Fun) {
		return nil, false
	}
	call, isCall := ps2108Unparen(conv.Args[0]).(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return nil, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Name() != "TrimSpace" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return nil, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return nil, false
	}
	inner, isConv := ps2108Unparen(call.Args[0]).(*ast.CallExpr)
	if !isConv || len(inner.Args) != 1 || inner.Ellipsis.IsValid() {
		return nil, false
	}
	if !ps2108IsByteSliceConv(pass, inner.Fun) {
		return nil, false
	}
	arg := inner.Args[0]
	tv, found := pass.TypesInfo.Types[arg]
	if !found || tv.Type == nil {
		return nil, false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		// A NAMED string operand is deliberately not matched:
		// strings.TrimSpace(s) would not compile for it, and the
		// semantics belong to the named type.
		return nil, false
	}
	return arg, true
}

// ps2012Fix builds the strings.TrimSpace(s) rewrite for one site, or nil
// when a guard fails and the report must stay advisory. Only the
// punctuation around the operand is replaced; the operand expression
// stays untouched in place, preserving its text and single evaluation
// (same technique as PS2110/PS3104). Import edits are appended later,
// once per file.
func ps2012Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr, s ast.Expr) *analysis.SuggestedFix {
	stringsName, usable := ps2012StringsName(pass, f, conv.Pos())
	if !usable {
		return nil
	}
	// The replaced spans are the punctuation around the operand; a
	// comment there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, conv.Pos(), s.Pos()) || ps2111CommentIn(f, s.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + stringsName + ".TrimSpace(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: s.Pos(), NewText: []byte(stringsName + ".TrimSpace(")},
			{Pos: s.End(), End: conv.End(), NewText: []byte(")")},
		},
	}
}

// ps2012StringsName resolves how the fix must spell the strings package
// at pos: the file's existing import name (alias included) when strings
// is already imported, or the bare name "strings" when the file must add
// the import. usable is false when the name cannot be used — a dot or
// blank strings import, or another object owning the name at pos.
func ps2012StringsName(pass *analysis.Pass, f *ast.File, pos token.Pos) (name string, usable bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"strings"` {
			continue
		}
		local := "strings"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			// A blank import gives no usable name; a dot import puts
			// TrimSpace in scope unqualified but rewriting to a bare
			// TrimSpace(s) is too fragile — advisory in both cases.
			return "", false
		}
		needImport, ok := ps2110PkgUsable(pass, pos, local, "strings")
		return local, ok && !needImport
	}
	needImport, ok := ps2110PkgUsable(pass, pos, "strings", "strings")
	return "strings", ok && needImport
}

// ps2012ImportEdits returns the import-block edits a file's combined
// fixes need: adding "strings" when missing (via the shared
// sorted-position insert PS2110/PS2112 use) and dropping the file's
// bytes import when the rewrites orphan it. When both apply and the swap
// keeps a gofmt-sorted group sorted, the bytes spec is replaced by
// "strings" in place — one edit; otherwise the bytes spec is removed and
// "strings" inserted at its own sorted position (the two edits never
// overlap). ok is false when the import block must not or cannot be
// edited (cgo file, missing bytes spec).
func ps2012ImportEdits(f *ast.File, needStrings, dropBytes bool) (edits []analysis.TextEdit, ok bool) {
	if !needStrings && !dropBytes {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropBytes {
		return []analysis.TextEdit{ps2110ImportEdit(f, "strings")}, true
	}
	gd, i, spec, found := ps2012FindImport(f, `"bytes"`)
	if !found {
		return nil, false
	}
	if !needStrings {
		return []analysis.TextEdit{ps2012RemoveSpec(gd, i, spec)}, true
	}
	// Swapping the bytes spec for "strings" in place keeps a sorted
	// group sorted only when no following path orders between "bytes"
	// and "strings" (unlike PS3104's sort->slices swap, many std paths
	// do). Swap when bytes is the sole or last spec, or the next path
	// already sorts after "strings"; otherwise remove + sorted insert.
	swapOK := i == len(gd.Specs)-1
	if !swapOK {
		if next, isSpec := gd.Specs[i+1].(*ast.ImportSpec); isSpec && next.Path != nil && next.Path.Value > `"strings"` {
			swapOK = true
		}
	}
	if swapOK {
		return []analysis.TextEdit{{Pos: spec.Pos(), End: spec.End(), NewText: []byte(`"strings"`)}}, true
	}
	return []analysis.TextEdit{ps2012RemoveSpec(gd, i, spec), ps2110ImportEdit(f, "strings")}, true
}

// ps2012FindImport locates the import spec with the given quoted path in
// f's import declarations.
func ps2012FindImport(f *ast.File, quoted string) (gd *ast.GenDecl, index int, spec *ast.ImportSpec, found bool) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		for i, s := range g.Specs {
			is, ok := s.(*ast.ImportSpec)
			if ok && is.Path != nil && is.Path.Value == quoted {
				return g, i, is, true
			}
		}
	}
	return nil, 0, nil, false
}

// ps2012RemoveSpec returns the edit removing one import spec from its
// declaration: the whole declaration when it is the sole spec, otherwise
// the spec plus the separating whitespace (same shape as PS3104's sort
// spec removal).
func ps2012RemoveSpec(gd *ast.GenDecl, i int, spec *ast.ImportSpec) analysis.TextEdit {
	switch {
	case len(gd.Specs) == 1:
		// Sole spec: drop the whole declaration (parenthesized or not);
		// gofmt normalizes the leftover blank line.
		return analysis.TextEdit{Pos: gd.Pos(), End: gd.End()}
	case i+1 < len(gd.Specs):
		return analysis.TextEdit{Pos: spec.Pos(), End: gd.Specs[i+1].Pos()}
	default:
		return analysis.TextEdit{Pos: gd.Specs[i-1].End(), End: spec.End()}
	}
}
