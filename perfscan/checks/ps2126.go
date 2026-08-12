package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2126 reports fmt.Errorf(msg) where msg is a compile-time string
// constant containing no '%' — a formatting call that formats nothing —
// and rewrites it to errors.New(msg).
var PS2126 = register(&lint.Check{
	ID:       "PS2126",
	Category: "alloc",
	Slug:     "errorf-const-to-errors-new",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Errorf on a constant verb-free message is errors.New plus a wasted formatter pass",
		Text: `fmt.Errorf without a %w wrapping verb is documented and implemented
as errors.New(fmt.Sprintf(format, args...)). For a constant format string
that contains no '%' and no further arguments, the Sprintf pass is a
byte-for-byte copy: the printer state is borrowed, the format string is
scanned rune by rune for verbs that are not there, and the result is the
same bytes back. errors.New(msg) skips all of it. The two spellings are
EQUIVALENT, not merely similar: both return a *errors.errorString whose
message is exactly the constant — the same dynamic type, the same
Error() text — so errors.Is/As chains, type assertions, and comparisons
behave identically (verified at runtime: reflect.TypeOf equal, messages
equal).

The '%' guard is what makes the equivalence provable. fmt treats every
'%' as a verb: fmt.Errorf("100%") yields "100%!(NOVERB)", not "100%",
so ANY '%' in the constant disqualifies the rewrite (this also excludes
%w, which would need a second argument anyway). The argument must have a
type-checker constant value of kind string — a string literal or a const
identifier both qualify; a variable does not, because its runtime
contents cannot be proven '%'-free. The callee is pinned with type
information to the package function fmt.Errorf: a shadowed fmt, a local
Errorf, or a method never matches. Calls with more than one argument or
a ... ellipsis do real formatting and are never flagged.

The fix rewrites only the call prefix — fmt.Errorf( becomes
errors.New( — keeping the argument text byte-verbatim, and edits the
file's imports once per file after collecting every fixable site: the
errors import is added when missing (reusing an existing alias when the
file imports errors under another name), and the fmt import is dropped
only when the rewrites remove the file's LAST fmt reference — any other
use of fmt (Sprintf, Println, a stored fmt.Errorf value, ...) keeps it.
When the same edit must both add errors and drop fmt, the fmt import
spec is swapped for "errors" in place. A dot- or blank-imported errors,
a site-local shadow of the errors name, a comment inside the rewritten
call prefix, or a cgo file (whose import block must not be edited) keeps
the report advisory.`,
		Before: `err := fmt.Errorf("connection closed")`,
		After:  `err := errors.New("connection closed")`,
		MeasuredWin: `BenchmarkPS2126 (constructing an error from a constant
18-byte message per op, Apple M2 Pro, go1.26.5): 15.4 ns/op before vs
13.1 ns/op after, 16 B/op and 1 alloc/op either way — ~15% faster.
go1.26's fmt has learned to hand the verbless single string through
without a second allocation, so on this toolchain the remaining cost is
the printer state borrow and the rune-by-rune verb scan that errors.New
skips; on older toolchains the Before arm also paid an intermediate
string allocation (2 allocs). The rewrite is also the idiomatic
spelling — go vet's printf checker and staticcheck both point the same
direction.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2126",
		Doc:  "fmt.Errorf on a constant verb-free string instead of errors.New",
		Run:  runPS2126,
	},
})

const ps2126Message = "fmt.Errorf on a constant verb-free message runs the whole fmt printer just to copy the string; errors.New returns the identical *errors.errorString without the printer allocation or format scan"

func runPS2126(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first, decide import edits once per file: every fixable
		// call removes one fmt reference, and whether the fmt import is
		// orphaned depends on ALL of them together (same per-file site
		// collection as PS3104).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package function
			// fmt.Errorf: a shadowed fmt resolves sel.Sel to some other
			// object, a local Errorf is not from package fmt, and a
			// method carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" || fn.Name() != "Errorf" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The single argument must be a compile-time string constant
			// (literal or const identifier) with no '%': fmt treats every
			// '%' as a verb — fmt.Errorf("100%") yields "100%!(NOVERB)" —
			// so any '%' breaks the equivalence, and a non-constant
			// argument cannot be proven '%'-free.
			tv, ok := pass.TypesInfo.Types[call.Args[0]]
			if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
				return true
			}
			if strings.ContainsRune(constant.StringVal(tv.Value), '%') {
				return true
			}
			fix := ps2126Fix(pass, f, call, call.Args[0])
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Each fixable call holds exactly one fmt reference (its selector's
		// package identifier); when those are ALL of the file's fmt
		// references, the rewrites orphan the import and the fix must drop
		// it (the runner never prunes imports itself). The errors import is
		// needed when the FILE lacks one — a site-local shadow only
		// suppresses that site's fix, never the file's import edit.
		errorsImported := false
		for _, imp := range f.Imports {
			if imp.Path.Value == `"errors"` {
				errorsImported = true
				break
			}
		}
		needImport := fixable > 0 && !errorsImported
		orphansFmt := fixable > 0 && pkgRefCount(pass, f, "fmt") == fixable
		importEdits, importsOK := ps2126ImportEdits(f, needImport, orphansFmt)
		if !importsOK {
			// cgo file needing import surgery, or a fmt spec we could not
			// locate: keep every report advisory.
			for i := range sites {
				sites[i].fix = nil
			}
		} else if len(importEdits) > 0 {
			// All fixes of a run are applied together, so only the first
			// fixable site carries the import edits (same convention as
			// PS3104).
			for i := range sites {
				if sites[i].fix != nil {
					sites[i].fix.TextEdits = append(sites[i].fix.TextEdits, importEdits...)
					break
				}
			}
		}
		for i := range sites {
			diag := analysis.Diagnostic{
				Pos:     sites[i].call.Pos(),
				End:     sites[i].call.End(),
				Message: ps2126Message,
			}
			if sites[i].fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*sites[i].fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2126Fix builds the errors.New(arg) rewrite for one call, or nil when a
// guard fails and the report must stay advisory. Only the call prefix up to
// the argument is replaced; the argument and the closing punctuation stay
// untouched in place, preserving the argument text byte-verbatim (same
// technique as PS3104). Import edits are appended later, once per file.
func ps2126Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, arg ast.Expr) *analysis.SuggestedFix {
	errorsName, _, usable := ps2126ErrorsName(pass, f, call.Pos())
	if !usable {
		return nil
	}
	// The replaced span is the call text before the argument; a comment
	// there would be silently destroyed — advisory then.
	if ps2111CommentIn(f, call.Pos(), arg.Pos()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace with " + errorsName + ".New(...)",
		TextEdits: []analysis.TextEdit{
			{Pos: call.Pos(), End: arg.Pos(), NewText: []byte(errorsName + ".New(")},
		},
	}
}

// ps2126ErrorsName resolves how the fix must spell the errors package at
// pos: the file's existing import name (alias included) when errors is
// already imported, or the bare name "errors" with needImport set when the
// file must add the import. usable is false when the name cannot be used —
// a dot or blank errors import, or another object owning the name at pos.
func ps2126ErrorsName(pass *analysis.Pass, f *ast.File, pos token.Pos) (name string, needImport, usable bool) {
	for _, imp := range f.Imports {
		if imp.Path.Value != `"errors"` {
			continue
		}
		local := "errors"
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if local == "_" || local == "." {
			// Blank import gives no usable name; a dot import puts New in
			// scope unqualified but rewriting to a bare New(msg) is too
			// fragile — advisory in both cases.
			return "", false, false
		}
		nf, ok := ps2110PkgUsable(pass, pos, local, "errors")
		return local, false, ok && !nf
	}
	needImport, usable = ps2110PkgUsable(pass, pos, "errors", "errors")
	return "errors", needImport, usable && needImport
}

// ps2126ImportEdits returns the import-block edits a file's combined fixes
// need: adding "errors" when missing (via the shared sorted-position insert
// PS2110/PS3104 use) and dropping the file's fmt import when the rewrites
// orphan it. When both apply, the fmt spec is replaced by "errors" in
// place — one edit, no overlap (the runner gofmt-formats the result, which
// re-sorts the group should some path order between "errors" and "fmt").
// ok is false when the import block must not or cannot be edited (cgo
// file, missing fmt spec).
func ps2126ImportEdits(f *ast.File, needErrors, dropFmt bool) (edits []analysis.TextEdit, ok bool) {
	if !needErrors && !dropFmt {
		return nil, true
	}
	if ps2110ImportsC(f) {
		return nil, false
	}
	if !dropFmt {
		return []analysis.TextEdit{ps2110ImportEdit(f, "errors")}, true
	}
	edit, found := ps2126FmtImportEdit(f, needErrors)
	if !found {
		return nil, false
	}
	return []analysis.TextEdit{edit}, true
}

// ps2126FmtImportEdit returns the edit removing the file's fmt import
// spec — or, when replaceWithErrors is set, swapping the whole spec (alias
// included) for "errors" in place.
func ps2126FmtImportEdit(f *ast.File, replaceWithErrors bool) (analysis.TextEdit, bool) {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for i, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok || is.Path == nil || is.Path.Value != `"fmt"` {
				continue
			}
			if replaceWithErrors {
				return analysis.TextEdit{Pos: is.Pos(), End: is.End(), NewText: []byte(`"errors"`)}, true
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
