package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2003 reports allocating strings transforms inside loops.
var PS2003 = register(&lint.Check{
	ID:       "PS2003",
	Category: "alloc",
	Slug:     "strings-alloc-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an allocating strings transform (Replace/Map/Repeat) in a loop",
		Text: `strings.Replace, ReplaceAll, Map and Repeat allocate a fresh
result string per call. In a loop that is one allocation (often more, via
internal growth) per iteration. Hoist the transform when its inputs are
loop-invariant; for repeated replacements build a strings.Replacer once; for
byte-level work use a reused []byte buffer.

The automatic fix hoists a call whose arguments are all provably
loop-invariant — basic literals, or plain identifiers that are not written
anywhere in the outermost enclosing loop (assigned, incremented,
redeclared, used as a range key/value, or address-taken) — by binding the
result immediately before the outermost loop and replacing the call with
the variable. Calls whose arguments mention loop state are reported
without a fix; whether those inputs are invariant is a data question left
to the reader.

strings.Repeat is hoisted only when its count argument is a provably
non-negative integer constant: strings.Repeat panics when count is
negative, and hoisting the call out of the loop would move that panic —
or, for a loop that runs zero times, introduce a panic the original never
raised. A variable count keeps the advisory report. strings.Replace,
ReplaceAll and Map never panic and are unaffected.`,
		Before: `for _, name := range names {
	clean := strings.ReplaceAll(name, "-", "_")
	emit(clean)
}`,
		After: `r := strings.NewReplacer("-", "_")
for _, name := range names {
	emit(r.Replace(name))
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2003",
		Doc:  "allocating strings transform in a loop",
		Run:  runPS2003,
	},
})

var stringsAllocFuncs = map[string]bool{
	"Replace":    true,
	"ReplaceAll": true,
	"Map":        true,
	"Repeat":     true,
}

func runPS2003(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "strings", stringsAllocFuncs)
			if !ok {
				return true
			}
			if _, inLoop := astutil.InLoop(stack); !inLoop {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "strings." + name + " in a loop allocates a fresh string per iteration; hoist the transform, build a strings.Replacer once, or reuse a byte buffer",
			}
			if fix := hoistStringsFix(pass, stack, call, name); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// hoistStringsFix builds the two-edit hoist: insert a binding before the
// outermost enclosing loop and replace the in-loop call with the variable.
// Eligible only when every argument is provably loop-invariant: a basic
// literal, or a plain identifier that is not written anywhere in the
// outermost enclosing loop (which covers iteration variables of every
// enclosing loop — they are assigned in the loop clause or ranged over as
// key/value). Anything else stays advisory.
//
// strings.Repeat additionally requires a provably non-negative constant
// count: Repeat panics when count < 0, and hoisting the call out of the
// loop relocates that panic — for a zero-iteration loop the original never
// evaluates Repeat (no panic) while the hoisted binding panics before the
// loop. A variable count cannot be proven non-negative, so it stays
// advisory. Replace/ReplaceAll/Map never panic and need no such gate.
func hoistStringsFix(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr, fnName string) *analysis.SuggestedFix {
	fset := pass.Fset
	if call.Ellipsis.IsValid() {
		return nil
	}
	loop, ok := astutil.OutermostLoop(stack)
	if !ok {
		return nil
	}
	if fnName == "Repeat" {
		if len(call.Args) != 2 {
			return nil
		}
		cv := pass.TypesInfo.Types[call.Args[1]].Value
		if cv == nil || cv.Kind() != constant.Int || constant.Sign(cv) < 0 {
			return nil
		}
	}
	args := make([]string, 0, len(call.Args))
	for _, a := range call.Args {
		switch x := a.(type) {
		case *ast.BasicLit:
			args = append(args, x.Value)
		case *ast.Ident:
			if identWrittenIn(loop, x.Name) {
				return nil
			}
			args = append(args, x.Name)
		default:
			return nil
		}
	}
	pos := fset.Position(call.Pos())
	name := fmt.Sprintf("psStr%d", pos.Line)
	loopPos := fset.Position(loop.Pos())
	// Assume gofmt indentation (tabs): the loop starts at column loopPos.Column,
	// i.e. loopPos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", loopPos.Column-1)
	// Render the qualifier from the source selector, not the literal "strings":
	// PkgFuncCall matches an aliased `import s "strings"` too, and the hoisted
	// binding must reuse the same alias (`s.ToUpper`) to compile bit-identically.
	// call.Fun is guaranteed a *ast.SelectorExpr here (PkgFuncCall matched it).
	sel := call.Fun.(*ast.SelectorExpr)
	binding := name + " := " + exprTextRendered(sel.X) + "." + fnName + "(" + strings.Join(args, ", ") + ")\n" + indent
	return &analysis.SuggestedFix{
		Message: "hoist strings." + fnName + " out of the loop",
		TextEdits: []analysis.TextEdit{
			{Pos: loop.Pos(), End: loop.Pos(), NewText: []byte(binding)},
			{Pos: call.Pos(), End: call.End(), NewText: []byte(name)},
		},
	}
}

// identWrittenIn reports whether the subtree rooted at root writes to name:
// assignment (including := and range key/value), increment/decrement, a var
// declaration (which would also shadow it), or taking its address. Scanning
// the whole outermost loop node — clause and body, nested loops and
// closures included — makes the check conservative: any write anywhere in
// the loop disqualifies the hoist.
func identWrittenIn(root ast.Node, name string) bool {
	found := false
	ast.Inspect(root, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := x.X.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{x.Key, x.Value} {
				if id, ok := e.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.DeclStmt:
			if gd, ok := x.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, id := range vs.Names {
							if id.Name == name {
								found = true
							}
						}
					}
				}
			}
		case *ast.UnaryExpr:
			if x.Op == token.AND && baseIdentName(x.X) == name {
				found = true
			}
		}
		return !found
	})
	return found
}
