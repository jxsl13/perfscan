package checks

import (
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2107 reports fmt.Sprintf calls whose format is exactly one verb over a
// single value with a direct strconv/hex equivalent.
var PS2107 = register(&lint.Check{
	ID:       "PS2107",
	Category: "alloc",
	Slug:     "sprintf-single-value",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Sprintf of a single value has a faster strconv/hex equivalent",
		Text: `fmt.Sprintf parses its format string, boxes the argument into an
interface, and dispatches through fmt's formatter state machine — even when
the format is a single bare verb over one value. The standard library has a
direct conversion for each such shape:

  fmt.Sprintf("%d", i)  -> strconv.Itoa(i)            (i is int)
                        -> strconv.FormatInt/FormatUint (other widths)
  fmt.Sprintf("%t", b)  -> strconv.FormatBool(b)
  fmt.Sprintf("%x", bs) -> hex.EncodeToString(bs)     (bs is []byte)

Only formats that are EXACTLY one of these verbs and nothing else are
reported, with a single non-variadic argument of the matching type.
(fmt.Sprintf("%s", s) over a string is an IDENTITY, not a conversion; that
belongs to PS2130 — reporting it here too would double-report the call.)

The automatic fix rewrites the call when the rewrite is provably
bit-identical: the argument's type must be the unnamed predeclared kind
(int/uint/... , bool, []byte, string) — a named type could implement
fmt.Formatter and change what fmt prints, so named types keep the plain
advisory report. The fix adds the strconv or encoding/hex import when the
file lacks it and is suppressed when the needed package name is shadowed
at the call site or when a cgo file would need the import added (a cgo
file's import block must not be edited). The fix also applies when the
rewrite removes the file's last fmt reference — the fix pipeline prunes
the now-unused fmt import — except in cgo files, where the orphan could
not be pruned, so the report stays advisory there.`,
		Before: `id := fmt.Sprintf("%d", userID)`,
		After:  `id := strconv.Itoa(userID)`,
		MeasuredWin: `strconv.Itoa runs several times faster than
fmt.Sprintf("%d", ...) and skips the interface boxing and fmt buffer
allocations (measured ~4x faster, 1 alloc instead of 2 for a 4-digit int).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2107",
		Doc:  "fmt.Sprintf of a single value with a direct strconv/hex equivalent",
		Run:  runPS2107,
	},
})

func runPS2107(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: applying all fixes may rewrite the file's last
		// fmt reference and orphan the import. The fix pipeline prunes
		// such an orphan afterwards — except in a cgo file (import "C"),
		// whose import block is never edited, so there the fixes are
		// withheld and the reports stay advisory.
		type site struct {
			call *ast.CallExpr
			msg  string
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		// All fixes of a run are applied together, so only the first fix
		// needing a given import carries the import edit (same convention
		// as PS2102's "strings" injection).
		importAdded := map[string]bool{}
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			if _, ok := astutil.PkgFuncCall(call.Fun, "fmt", map[string]bool{"Sprintf": true}); !ok {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			verb, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			c := ps2107Classify(pass, verb, call.Args[1])
			if c == nil {
				return true
			}
			var fix *analysis.SuggestedFix
			if c.repl != "" && !ps2107CommentsOverlap(f, call) {
				needImport, usable := true, true
				if c.pkgName != "" {
					needImport, usable = ps2107PkgUsable(pass, call.Pos(), c.pkgName, c.pkgPath)
				} else {
					needImport = false
				}
				if usable && !(needImport && ps2107ImportsC(f)) {
					edits := []analysis.TextEdit{
						{Pos: call.Pos(), End: call.End(), NewText: []byte(c.repl)},
					}
					if needImport && !importAdded[c.pkgPath] {
						edits = append(edits, ps2107ImportEdit(f, c.pkgPath))
						importAdded[c.pkgPath] = true
					}
					fix = &analysis.SuggestedFix{
						Message:   "replace fmt.Sprintf(" + lit.Value + ", ...) with " + c.replName,
						TextEdits: edits,
					}
					fixable++
				}
			}
			sites = append(sites, site{call, c.msg, fix})
			return true
		})
		emitFixes := fixable > 0 && (pkgRefCount(pass, f, "fmt") > fixable || !ps2110ImportsC(f))
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: st.msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2107Case is one recognized single-verb shape: the diagnostic message
// and, when the rewrite is provably bit-identical, the replacement
// expression and the package it references.
type ps2107Case struct {
	msg      string
	repl     string // replacement text; "" keeps the report advisory
	replName string // human name of the replacement, for the fix message
	pkgName  string // identifier the replacement references ("strconv", "hex")
	pkgPath  string // its import path
}

// ps2107Classify matches verb+argument against the four recognized shapes.
// It returns nil when the call is not one of them (wrong verb, wrong
// argument type). A shape match with a NAMED argument type yields an
// advisory-only case (repl == ""): a named type may implement
// fmt.Formatter, which fmt would honor and the direct conversion would not.
// Unnamed basic kinds ([]byte for %x) cannot carry methods, so for them the
// rewrite is bit-identical.
func ps2107Classify(pass *analysis.Pass, verb string, arg ast.Expr) *ps2107Case {
	t := pass.TypesInfo.TypeOf(arg)
	if t == nil {
		return nil
	}
	// An untyped constant materializes as its default type when passed to
	// Sprintf's ...any parameter.
	t = types.Default(t)
	basic, tIsBasic := t.(*types.Basic)
	under, underBasic := t.Underlying().(*types.Basic)
	const boxes = " boxes the argument and walks fmt's formatter state machine; "
	switch verb {
	case "%d":
		if !underBasic || under.Info()&types.IsInteger == 0 {
			return nil
		}
		c := &ps2107Case{msg: "fmt.Sprintf of a single %d value" + boxes + "strconv.Itoa/FormatInt/FormatUint converts it directly"}
		if !tIsBasic {
			return c
		}
		argText, ok := ps2107ExprText(arg)
		if !ok {
			return c
		}
		switch {
		case basic.Kind() == types.Int:
			c.repl = "strconv.Itoa(" + argText + ")"
			c.replName = "strconv.Itoa"
		case basic.Info()&types.IsUnsigned != 0:
			c.repl = "strconv.FormatUint(uint64(" + argText + "), 10)"
			c.replName = "strconv.FormatUint"
		default:
			c.repl = "strconv.FormatInt(int64(" + argText + "), 10)"
			c.replName = "strconv.FormatInt"
		}
		c.pkgName, c.pkgPath = "strconv", "strconv"
		return c
	case "%t":
		if !underBasic || under.Info()&types.IsBoolean == 0 {
			return nil
		}
		c := &ps2107Case{msg: "fmt.Sprintf of a single %t value" + boxes + "strconv.FormatBool converts it directly"}
		if !tIsBasic {
			return c
		}
		argText, ok := ps2107ExprText(arg)
		if !ok {
			return c
		}
		c.repl = "strconv.FormatBool(" + argText + ")"
		c.replName = "strconv.FormatBool"
		c.pkgName, c.pkgPath = "strconv", "strconv"
		return c
	case "%x":
		// Only byte slices: %x over strings or integers is out of scope.
		uSl, uIsSlice := t.Underlying().(*types.Slice)
		if !uIsSlice {
			return nil
		}
		// The element must be the predeclared byte itself — a named byte
		// element would make the slice unassignable to hex's []byte.
		eb, ok := uSl.Elem().(*types.Basic)
		if !ok || eb.Kind() != types.Byte {
			return nil
		}
		c := &ps2107Case{msg: "fmt.Sprintf of a single %x []byte value" + boxes + "hex.EncodeToString converts it directly"}
		if _, unnamed := t.(*types.Slice); !unnamed {
			return c
		}
		argText, ok := ps2107ExprText(arg)
		if !ok {
			return c
		}
		c.repl = "hex.EncodeToString(" + argText + ")"
		c.replName = "hex.EncodeToString"
		c.pkgName, c.pkgPath = "hex", "encoding/hex"
		return c
		// NOTE: %s over a string is an IDENTITY (fmt.Sprintf("%s", s) -> s), not
		// a conversion like the cases above; it is owned by PS2130 (which also
		// covers "%v" and fmt.Sprint and carries the composite-literal-in-header
		// paren guard), so PS2107 deliberately does not handle it — that would
		// double-report and double-fix the same call.
	}
	return nil
}

// ps2107ExprText renders arg for splicing into the replacement.
func ps2107ExprText(arg ast.Expr) (string, bool) {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), arg); err != nil {
		return "", false
	}
	return b.String(), true
}

// ps2107CommentsOverlap reports whether a comment overlaps the replaced
// call range — the re-rendered replacement would not reproduce it.
func ps2107CommentsOverlap(f *ast.File, call *ast.CallExpr) bool {
	for _, cg := range f.Comments {
		if cg.End() > call.Pos() && cg.Pos() < call.End() {
			return true
		}
	}
	return false
}

// ps2107PkgUsable reports whether name can reference the package at path
// from pos: needImport is true when the file must add the import, ok is
// false when some other object owns the name there (a local or
// package-level declaration, or a same-named other package).
func ps2107PkgUsable(pass *analysis.Pass, pos token.Pos, name, path string) (needImport, ok bool) {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		return false, false
	}
	if _, obj := scope.LookupParent(name, pos); obj != nil {
		pn, isPkg := obj.(*types.PkgName)
		return false, isPkg && pn.Imported().Path() == path
	}
	return true, true
}

// ps2107ImportsC reports whether f is a cgo file: its import block must
// never be edited, a misplaced insertion could detach the preamble from
// import "C".
func ps2107ImportsC(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"C"` {
			return true
		}
	}
	return false
}

// ps2107ImportEdit returns the edit adding `import "path"` to f: into the
// first import group at its sorted position, above a single import
// declaration, or after the package clause when the file has no imports.
func ps2107ImportEdit(f *ast.File, path string) analysis.TextEdit {
	quoted := strconv.Quote(path)
	var group *ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			group = gd
			break
		}
	}
	if group == nil {
		pos := f.Name.End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\nimport " + quoted)}
	}
	if !group.Lparen.IsValid() {
		pos := group.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("import " + quoted + "\n")}
	}
	for _, spec := range group.Specs {
		is, ok := spec.(*ast.ImportSpec)
		if !ok || is.Path == nil || is.Path.Value <= quoted {
			continue
		}
		pos := spec.Pos()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(quoted + "\n\t")}
	}
	if n := len(group.Specs); n > 0 {
		pos := group.Specs[n-1].End()
		return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t" + quoted)}
	}
	pos := group.Lparen + 1
	return analysis.TextEdit{Pos: pos, End: pos, NewText: []byte("\n\t" + quoted)}
}
