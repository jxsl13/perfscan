package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3004 reports bytes.Contains([]byte(s), []byte(sub)) and friends — a
// read-only bytes predicate fed freshly-converted strings — where the
// strings twin scans the same bytes with zero allocations.
var PS3004 = register(&lint.Check{
	ID:       "PS3004",
	Category: "alloc",
	Slug:     "bytes-predicate-on-converted-strings",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a bytes.* predicate fed []byte(s) conversions of strings pays two throwaway copies; the strings.* twin scans the strings directly",
		Text: `bytes.Contains([]byte(s), []byte(sub)) with s and sub plain
strings materializes two throwaway []byte copies of the operands on EVERY
call — two allocations plus two memmoves — before the actual scan even
starts. strings.Contains(s, sub) runs the identical algorithm over the
identical bytes straight off the string headers: zero allocations, same
asymptotics, strictly less work per call.

The rewrite is BIT-IDENTICAL. For any string x, []byte(x) is a fresh
slice holding exactly x's bytes (round-trip is byte-exact, invalid UTF-8
included), and each matched bytes.* member is specified to operate on the
raw bytes with the same algorithm as its strings.* twin — the bool or int
result is therefore identical for every input, empty strings and empty
separators included (pinned by the equivalence suite over adversarial
inputs). Each operand expression is kept byte-verbatim in place —
evaluated exactly once, in the original order; only the surrounding
scaffolding (the callee and the conversions) is rewritten.

Only members whose strings twin has the SAME parameter and result shape
are matched: Contains, HasPrefix, HasSuffix, Index, LastIndex, Count and
EqualFold (both parameters []byte, so BOTH arguments must be []byte(x)
conversions), plus ContainsAny and IndexAny (whose second parameter is
already a string in both packages, so only the first argument is a
conversion and the second carries over verbatim). Everything else is out
of scope: IndexByte/IndexRune/ContainsRune take a byte or rune, Equal is
the == operator on strings, and Split/Fields/TrimPrefix-style members
RETURN derived []byte values — a different pattern entirely.

The match is deliberately strict. The callee is resolved with type
information — only the standard library's package-level bytes functions
match, never a shadowed identifier or a same-named third-party package
(an aliased bytes import still matches). Each rewritten argument must be
a conversion to exactly the predeclared unnamed []byte whose operand is
statically the predeclared string type (or an untyped string constant):
a defined string type, a defined byte-slice conversion target, or a raw
[]byte argument would either not compile against the strings twin or is
simply a different pattern, and none match.

The fix edits imports as needed: the strings import is added when
missing, and the bytes import is dropped when the rewrites remove the
file's LAST bytes reference — when they replace the whole import, the
bytes spec is swapped for "strings" in place. A dot-, blank- or
alias-imported strings, a local declaration shadowing the strings name at
the call site, a comment inside the rewritten scaffolding, a file
importing bytes under more than one name, or a cgo file (whose import
block must not be edited) keeps the report advisory.`,
		Before: `if bytes.Contains([]byte(s), []byte(sub)) { ... }`,
		After:  `if strings.Contains(s, sub) { ... }`,
		MeasuredWin: `BenchmarkPS3004 (60-byte haystack, 9-byte needle near
the end, Apple M2 Pro, go1.26): bytes.Index([]byte(s), []byte(sub))
54.8 ns/op 64 B/op 1 alloc -> strings.Index(s, sub) 25.2 ns/op 0 B/op
0 allocs (~2.2x; the alloc is the throwaway haystack copy — gc keeps the
small non-escaping needle copy on the stack here, larger or escaping
operands pay the second allocation too).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3004",
		Doc:  "bytes.* predicate over []byte(string) conversions instead of the allocation-free strings.* twin",
		Run:  runPS3004,
	},
})

// ps3004Members are the bytes package members whose strings twin takes the
// same argument shape and returns the same bool/int result over the same
// bytes. The value records whether the SECOND parameter is []byte too
// (true: both arguments must be []byte(x) conversions) or already a string
// in both packages (false: the second argument carries over verbatim).
var ps3004Members = map[string]bool{
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

func runPS3004(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one bytes reference, and whether the bytes import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS2129/PS2118).
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
			name, first, second, bothConv, ok := ps3004Match(pass, call)
			if !ok {
				return true
			}
			fix := ps3004Fix(pass, f, call, name, first, second)
			if fix != nil {
				fixable++
			}
			var msg string
			if bothConv {
				msg = "bytes." + name + "([]byte(s), []byte(sub)) allocates two throwaway []byte copies just to scan them; strings." + name + "(s, sub) runs the same scan on the string bytes directly with zero allocations"
			} else {
				msg = "bytes." + name + "([]byte(s), chars) allocates a throwaway []byte copy just to scan it; strings." + name + "(s, chars) runs the same scan on the string bytes directly with zero allocations"
			}
			sites = append(sites, site{call, msg, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one bytes reference (its selector's
		// package identifier); when those are ALL of the file's bytes
		// references, the rewrites orphan the import and the fix must drop it
		// (the runner never prunes imports itself). The strings import is
		// needed when the FILE lacks a plain one — a site-local shadow only
		// suppresses that site's fix, never the file's import edit.
		stringsImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"strings"` && imp.Name == nil {
				stringsImported = true
				break
			}
		}
		needImport := fixable > 0 && !stringsImported
		orphansBytes := fixable > 0 && pkgRefCount(pass, f, "bytes") == fixable
		importEdits, importsOK := ps3004ImportEdits(f, needImport, orphansBytes)
		// A file importing "bytes" under more than one name (legal but
		// pathological) breaks the name-blind bytes ref count and the
		// first-match spec selection: rewriting a call reached through one
		// name can orphan the OTHER spec, yielding "imported and not used".
		// Refuse to fix any site in such a file — advisory only (same guard
		// as PS2129's multi-fmt case).
		if fixable > 0 && ps3004MultipleBytesSpecs(f) {
			importsOK = false
		}
		if !importsOK {
			// cgo file needing import surgery, a bytes spec we could not
			// locate, or a multi-spec bytes import: keep every report
			// advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS2129/PS3104).
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

// ps3004ByteSlice is the predeclared unnamed []byte the rewritten-away
// conversions must target exactly.
var ps3004ByteSlice = types.NewSlice(types.Typ[types.Byte])

// ps3004Match matches call as bytes.<member>(args) with every rewritten
// argument a []byte(x) conversion of a plain string (see ps3004Members for
// which arguments those are). The callee is pinned by type information to
// the standard library bytes package's package functions, so a shadowed
// bytes, a same-named third-party package or a method never matches. It
// returns the member name, the two operand expressions that survive the
// rewrite verbatim, and whether BOTH were conversions.
func ps3004Match(pass *analysis.Pass, call *ast.CallExpr) (name string, first, second ast.Expr, bothConv, ok bool) {
	if call.Ellipsis.IsValid() || len(call.Args) != 2 {
		return "", nil, nil, false, false
	}
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	if !isSel {
		return "", nil, nil, false, false
	}
	// The qualifier must BE the imported bytes package, not a value whose
	// evaluation the rewrite would drop.
	xid, isIdent := sel.X.(*ast.Ident)
	if !isIdent {
		return "", nil, nil, false, false
	}
	if pn, isPkg := pass.TypesInfo.Uses[xid].(*types.PkgName); !isPkg || pn.Imported().Path() != "bytes" {
		return "", nil, nil, false, false
	}
	fn, isFunc := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !isFunc || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return "", nil, nil, false, false
	}
	if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
		return "", nil, nil, false, false
	}
	secondIsBytes, listed := ps3004Members[fn.Name()]
	if !listed {
		return "", nil, nil, false, false
	}
	first, ok = ps3004ConvOperand(pass, call.Args[0])
	if !ok {
		return "", nil, nil, false, false
	}
	// The second parameter of ContainsAny/IndexAny is already a string in
	// BOTH packages — the argument carries over verbatim, and it typechecks
	// against the strings twin because the parameter types are identical.
	second = call.Args[1]
	if secondIsBytes {
		second, ok = ps3004ConvOperand(pass, call.Args[1])
		if !ok {
			return "", nil, nil, false, false
		}
	}
	return fn.Name(), first, second, secondIsBytes, true
}

// ps3004ConvOperand matches e as a conversion []byte(x) to exactly the
// predeclared unnamed []byte whose operand x is statically the predeclared
// string type or an untyped string constant — the only shapes for which
// dropping the conversion both compiles against the strings twin and feeds
// it the identical bytes. A defined byte-slice target or a defined string
// operand keeps PS3004 silent: the former is a different value, the latter
// would not compile as the strings.* parameter without a conversion.
func ps3004ConvOperand(pass *analysis.Pass, e ast.Expr) (ast.Expr, bool) {
	conv, ok := ps2110Unparen(e).(*ast.CallExpr)
	if !ok || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	tv, ok := pass.TypesInfo.Types[conv.Fun]
	if !ok || !tv.IsType() || !types.Identical(tv.Type, ps3004ByteSlice) {
		return nil, false
	}
	op := conv.Args[0]
	t := pass.TypesInfo.TypeOf(op)
	if t == nil {
		return nil, false
	}
	if !types.Identical(t, types.Typ[types.String]) && !types.Identical(t, types.Typ[types.UntypedString]) {
		return nil, false
	}
	return op, true
}

// ps3004Fix builds the strings.<name>(first, second) rewrite for one call,
// or nil when a guard fails and the report must stay advisory. Only the
// scaffolding around the two operands is replaced; their text stays
// untouched in place, preserving their single evaluation and order (same
// technique as PS2129/PS3104). Import edits are appended later, once per
// file.
func ps3004Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, name string, first, second ast.Expr) *analysis.SuggestedFix {
	if !ps3004StringsUsable(pass, f, call.Pos()) {
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
		Message: "replace with strings." + name + "(s, sub)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: first.Pos(), NewText: []byte("strings." + name + "(")},
			{Pos: first.End(), End: second.Pos(), NewText: []byte(", ")},
			{Pos: second.End(), End: call.End(), NewText: []byte(")")},
		},
	}
}

// ps3004StringsUsable reports whether the fix may spell strings.<name> at
// pos: the file's strings import must be un-renamed (the emitted text is
// exactly "strings.<name>", so a dot, blank or alias import keeps the
// report advisory), or absent and addable, and no other object may own the
// strings name at the call site.
func ps3004StringsUsable(pass *analysis.Pass, f *ast.File, pos token.Pos) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"strings"` {
			continue
		}
		if imp.Name != nil {
			// Renamed (alias, dot or blank): the bare strings qualifier the
			// fix emits would not resolve to the package — advisory.
			return false
		}
		needImport, ok := ps2110PkgUsable(pass, pos, "strings", "strings")
		return ok && !needImport
	}
	needImport, ok := ps2110PkgUsable(pass, pos, "strings", "strings")
	return ok && needImport
}

// ps3004MultipleBytesSpecs reports whether f imports the bytes package
// under more than one import spec. The rewrite's import bookkeeping
// assumes a single bytes spec; more than one makes both the ref count and
// the spec choice unreliable, so PS3004 declines to fix (advisory).
func ps3004MultipleBytesSpecs(f *ast.File) bool {
	n := 0
	for _, imp := range f.Imports {
		if imp.Path != nil && imp.Path.Value == `"bytes"` {
			n++
		}
	}
	return n > 1
}

// ps3004ImportEdits returns the import-block edits a file's combined fixes
// need: adding "strings" when missing (via the shared sorted-position
// insert PS2110/PS3104 use) and dropping the bytes import when the
// rewrites orphan it. When both apply, the bytes spec is replaced by
// "strings" in place — one edit, no overlap. ok is false when the import
// block must not or cannot be edited (cgo file, missing bytes spec).
func ps3004ImportEdits(f *ast.File, needStrings, dropBytes bool) (edits []analysis.TextEdit, ok bool) {
	if !needStrings && !dropBytes {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropBytes {
		return []analysis.TextEdit{ps2110ImportEdit(f, "strings")}, true
	}
	edit, found := ps3004BytesImportEdit(f, needStrings)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps3004BytesImportEdit returns the edit removing the file's bytes import
// spec — or, when replaceWithStrings is set, swapping the whole spec
// (alias included) for "strings" in place. Std paths order between "bytes"
// and "strings" (cmp, fmt, io, sort, …), so the swapped group may need a
// gofmt pass to re-sort — it always compiles (same note as PS2129's swap).
func ps3004BytesImportEdit(f *ast.File, replaceWithStrings bool) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"bytes"` {
				continue
			}
			if replaceWithStrings {
				return analysis.TextEdit{Pos: is.Pos(), End: is.End(), NewText: []byte(`"strings"`)}, true
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
