package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2019 reports strings.Contains(string(b), string(sub)) and friends — a
// read-only strings predicate fed freshly-converted byte slices — where
// the bytes twin scans the same bytes with zero allocations. It is the
// exact mirror of PS3004 (bytes predicate over []byte(string)
// conversions), run in the opposite direction.
var PS2019 = register(&lint.Check{
	ID:       "PS2019",
	Category: "alloc",
	Slug:     "strings-predicate-on-converted-bytes",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a strings.* predicate fed string([]byte) conversions pays throwaway string copies; the bytes.* twin scans the []byte directly",
		Text: `strings.Contains(string(b), string(sub)) with b and sub plain
[]byte values materializes two throwaway string copies of the operands on
EVERY call — two allocations plus two memmoves — before the actual scan
even starts. bytes.Contains(b, sub) runs the identical algorithm over the
identical bytes straight off the slice headers: zero allocations, same
asymptotics, strictly less work per call.

The rewrite is BIT-IDENTICAL. For any []byte b, string(b) is a fresh
string holding exactly b's bytes (round-trip is byte-exact, invalid UTF-8
included, and a nil slice converts to ""), and each matched strings.*
member is specified to operate on the raw bytes with the same algorithm
as its bytes.* twin — the bool or int result is therefore identical for
every input, empty operands and empty separators included (pinned by the
equivalence suite over adversarial inputs). None of the matched members
invokes user code, so in a race-free program nothing can observe that the
scan now reads the live slice instead of a snapshot. Each operand
expression is kept byte-verbatim in place — evaluated exactly once, in
the original order; only the surrounding scaffolding (the callee and the
conversions) is rewritten.

Only members whose bytes twin has the SAME parameter and result shape
are matched: Contains, HasPrefix, HasSuffix, Index, LastIndex, Count and
EqualFold (both bytes parameters []byte, so BOTH arguments must be
string(b) conversions), plus ContainsAny and IndexAny (whose second
parameter is a string in both packages, so only the first argument is a
conversion and the second carries over verbatim). Everything else is out
of scope: IndexByte/IndexRune/ContainsRune take a byte or rune, and
Split/Fields/TrimPrefix-style members RETURN derived strings — a
different pattern entirely.

The match is deliberately strict. The callee is resolved with type
information — only the standard library's package-level strings functions
match, never a shadowed identifier or a same-named third-party package
(an aliased strings import still matches). Each rewritten argument must
be a conversion to exactly the predeclared string type whose operand is
statically the predeclared unnamed []byte: a defined byte-slice operand
or a defined string conversion target would let defined-type semantics
leak into the bytes twin, so neither matches.

The fix edits imports as needed: the bytes import is added when missing,
and the strings import is dropped when the rewrites remove the file's
LAST strings reference — when they replace the whole import, the strings
spec is swapped for "bytes" in place. A dot-, blank- or alias-imported
bytes, a local declaration shadowing the bytes name at the call site, a
comment inside the rewritten scaffolding, a file importing strings under
more than one name, or a cgo file (whose import block must not be edited)
keeps the report advisory.`,
		Before: `if strings.Contains(string(b), string(sub)) { ... }`,
		After:  `if bytes.Contains(b, sub) { ... }`,
		MeasuredWin: `BenchmarkPS2019 (60-byte haystack, 9-byte needle near
the end, Apple M2 Pro, go1.26): strings.Index(string(b), string(sub))
39.0 ns/op 64 B/op 1 alloc -> bytes.Index(b, sub) 26.0 ns/op 0 B/op
0 allocs (~1.5x; the alloc is the throwaway haystack copy — gc keeps the
small non-escaping needle copy on the stack here, larger or escaping
operands pay the second allocation too).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2019",
		Doc:  "strings.* predicate over string([]byte) conversions instead of the allocation-free bytes.* twin",
		Run:  runPS2019,
	},
})

// ps2019Members are the strings package members whose bytes twin takes the
// same argument shape and returns the same bool/int result over the same
// bytes. The value records whether the SECOND parameter is []byte in the
// bytes twin (true: both arguments must be string(b) conversions) or a
// string in both packages (false: the second argument carries over
// verbatim).
var ps2019Members = map[string]bool{
	"Contains":    true,
	"ContainsAny": false,
	"HasPrefix":   true,
	"HasSuffix":   true,
	"Index":       true,
	"IndexAny":    false,
	"LastIndex":   true,
	"Count":       true,
	"EqualFold":   true,
}

func runPS2019(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one strings reference, and whether the strings import
		// is orphaned depends on ALL of them together (same per-file site
		// collection as PS3004/PS2129).
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, first, second, bothConv, ok := ps2019Match(pass, call)
			if !ok {
				return true
			}
			fix := ps2019Fix(pass, f, call, name, first, second)
			if fix != nil {
				fixable++
			}
			var msg string
			if bothConv {
				msg = "strings." + name + "(string(b), string(sub)) allocates two throwaway string copies just to scan them; bytes." + name + "(b, sub) runs the same scan on the bytes directly with zero allocations"
			} else {
				msg = "strings." + name + "(string(b), chars) allocates a throwaway string copy just to scan it; bytes." + name + "(b, chars) runs the same scan on the bytes directly with zero allocations"
			}
			sites = append(sites, site{call, msg, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one strings reference (its
		// selector's package identifier); when those are ALL of the file's
		// strings references, the rewrites orphan the import and the fix must
		// drop it (the runner never prunes imports itself). The bytes import
		// is needed when the FILE lacks a plain one — a site-local shadow
		// only suppresses that site's fix, never the file's import edit.
		bytesImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"bytes"` && imp.Name == nil {
				bytesImported = true
				break
			}
		}
		needImport := fixable > 0 && !bytesImported
		orphansStrings := fixable > 0 && pkgRefCount(pass, f, "strings") == fixable
		importEdits, importsOK := ps2019ImportEdits(f, needImport, orphansStrings)
		// A file importing "strings" under more than one name (legal but
		// pathological) breaks the name-blind strings ref count and the
		// first-match spec selection: rewriting a call reached through one
		// name can orphan the OTHER spec, yielding "imported and not used".
		// Refuse to fix any site in such a file — advisory only (same guard
		// as PS3004's multi-bytes case).
		if fixable > 0 && ps2019MultipleStringsSpecs(f) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing import surgery, a strings spec we could not
			// locate, or a multi-spec strings import: keep every report
			// advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS3004/PS2129).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: st.msg,
			}
			if st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2019ByteSlice is the predeclared unnamed []byte the rewritten-away
// conversions' operands must have exactly.
var ps2019ByteSlice = types.NewSlice(types.Typ[types.Byte])

// ps2019Match matches call as strings.<member>(args) with every rewritten
// argument a string(b) conversion of a plain []byte (see ps2019Members for
// which arguments those are). The callee is pinned by type information to
// the standard library strings package's package functions, so a shadowed
// strings, a same-named third-party package or a method never matches. It
// returns the member name, the two operand expressions that survive the
// rewrite verbatim, and whether BOTH were conversions.
func ps2019Match(pass *analysis.Pass, call *ast.CallExpr) (name string, first, second ast.Expr, bothConv, ok bool) {
	if call.Ellipsis.IsValid() || len(call.Args) != 2 {
		return "", nil, nil, false, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return "", nil, nil, false, false
	}
	// The qualifier must BE the imported strings package, not a value whose
	// evaluation the rewrite would drop.
	xid, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", nil, nil, false, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "strings" {
		return "", nil, nil, false, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
		return "", nil, nil, false, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return "", nil, nil, false, false
	}
	secondIsBytes, listed := ps2019Members[fn.Name()]
	if !listed {
		return "", nil, nil, false, false
	}
	first, ok = ps2019ConvOperand(pass, call.Args[0])
	if !ok {
		return "", nil, nil, false, false
	}
	// The second parameter of ContainsAny/IndexAny is a string in BOTH
	// packages — the argument carries over verbatim, and it typechecks
	// against the bytes twin because the parameter types are identical.
	second = call.Args[1]
	if secondIsBytes {
		second, ok = ps2019ConvOperand(pass, call.Args[1])
		if !ok {
			return "", nil, nil, false, false
		}
	}
	return fn.Name(), first, second, secondIsBytes, true
}

// ps2019ConvOperand matches e as a conversion string(b) to exactly the
// predeclared string type whose operand b is statically the predeclared
// unnamed []byte — the only shape for which dropping the conversion both
// compiles against the bytes twin and feeds it the identical bytes. A
// defined string conversion target cannot occur here in compiling code
// (the strings.* parameter is plain string), and a defined byte-slice
// operand keeps PS2019 silent: rewriting it would let defined-type
// semantics leak into the bytes twin.
func ps2019ConvOperand(pass *analysis.Pass, e ast.Expr) (ast.Expr, bool) {
	conv, ok := ps2110Unparen(e).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	tv, ok := pass.TypesInfo.Types[conv.Fun]
	if !ok || !tv.IsType() || !types.Identical(tv.Type, types.Typ[types.String]) {
		return nil, false
	}
	op := conv.Args[0]
	t := pass.TypesInfo.TypeOf(op)
	if t == nil || !types.Identical(t, ps2019ByteSlice) {
		return nil, false
	}
	return op, true
}

// ps2019Fix builds the bytes.<name>(first, second) rewrite for one call,
// or nil when a guard fails and the report must stay advisory. Only the
// scaffolding around the two operands is replaced; their text stays
// untouched in place, preserving their single evaluation and order (same
// technique as PS3004/PS2129). Import edits are appended later, once per
// file.
func ps2019Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, name string, first, second ast.Expr) *analysis.SuggestedFix {
	if !ps2019BytesUsable(pass, f, call.Pos()) {
		return nil
	}
	// The replaced spans are the call text around the operands (the
	// conversions included); a comment there would be silently destroyed —
	// advisory then.
	if ps2111CommentIn(f, call.Pos(), first.Pos()) ||
		ps2111CommentIn(f, first.End(), second.Pos()) ||
		ps2111CommentIn(f, second.End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with bytes." + name + "(b, sub)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: first.Pos(), NewText: []byte("bytes." + name + "(")},
			{Pos: first.End(), End: second.Pos(), NewText: []byte(", ")},
			{Pos: second.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps2019BytesUsable reports whether the fix may spell bytes.<name> at
// pos: the file's bytes import must be un-renamed (the emitted text is
// exactly "bytes.<name>", so a dot, blank or alias import keeps the
// report advisory), or absent and addable, and no other object may own
// the bytes name at the call site.
func ps2019BytesUsable(pass *analysis.Pass, f *ast.File, pos token.Pos) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"bytes"` {
			continue
		}
		if imp.Name != nil {
			// Renamed (alias, dot or blank): the bare bytes qualifier the
			// fix emits would not resolve to the package — advisory.
			return false
		}
		needImport, ok := ps2110PkgUsable(pass, pos, "bytes", "bytes")
		return ok && !needImport
	}
	needImport, ok := ps2110PkgUsable(pass, pos, "bytes", "bytes")
	return ok && needImport
}

// ps2019MultipleStringsSpecs reports whether f imports the strings package
// under more than one import spec. The rewrite's import bookkeeping
// assumes a single strings spec; more than one makes both the ref count
// and the spec choice unreliable, so PS2019 declines to fix (advisory).
func ps2019MultipleStringsSpecs(f *ast.File) bool {
	n := 0
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"strings"` {
			n++
		}
	}
	return n > 1
}

// ps2019ImportEdits returns the import-block edits a file's combined fixes
// need: adding "bytes" when missing (via the shared sorted-position insert
// PS2110/PS3104 use) and dropping the strings import when the rewrites
// orphan it. When both apply, the strings spec is replaced by "bytes" in
// place — one edit, no overlap. ok is false when the import block must not
// or cannot be edited (cgo file, missing strings spec).
func ps2019ImportEdits(f *ast.File, needBytes, dropStrings bool) (edits []analysis.TextEdit, ok bool) {
	if !needBytes && !dropStrings {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropStrings {
		return []analysis.TextEdit{ps2110ImportEdit(f, "bytes")}, true
	}
	edit, found := ps2019StringsImportEdit(f, needBytes)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps2019StringsImportEdit returns the edit removing the file's strings
// import spec — or, when replaceWithBytes is set, swapping the whole spec
// (alias included) for "bytes" in place. Std paths order between "bytes"
// and "strings" (cmp, fmt, io, sort, …), so the swapped group may need a
// gofmt pass to re-sort — it always compiles (same note as PS3004's swap).
func ps2019StringsImportEdit(f *ast.File, replaceWithBytes bool) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"strings"` {
				continue
			}
			if replaceWithBytes {
				return analysis.TextEdit{Pos: is.Pos(), End: is.End(), NewText: []byte(`"bytes"`)}, true
			}
			switch {
			case len(gd.Specs) == 1:
				// Sole spec: drop the whole declaration (parenthesized or
				// not); gofmt normalizes the leftover blank line.
				return analysis.TextEdit{Pos: gd.Pos(), End: gd.End()}, true
			case i+1 < len(gd.Specs):
				return analysis.TextEdit{Pos: is.Pos(), End: gd.Specs[i+1].Pos()}, true
			default:
				return analysis.TextEdit{Pos: gd.Specs[i-1].End(), End: is.End()}, true
			}
		}
	}
	return analysis.TextEdit{}, false
}
