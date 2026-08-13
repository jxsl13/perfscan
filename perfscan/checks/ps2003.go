package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
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
to the reader.`,
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
				Message: fmt.Sprintf("strings.%s in a loop allocates a fresh string per iteration; hoist the transform, build a strings.Replacer once, or reuse a byte buffer", name),
			}
			if fix := hoistStringsFix(pass.Fset, stack, call, name); fix != nil {
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
func hoistStringsFix(fset *token.FileSet, stack []ast.Node, call *ast.CallExpr, fnName string) *analysis.SuggestedFix {
	if call.Ellipsis.IsValid() {
		return nil
	}
	loop, ok := astutil.OutermostLoop(stack)
	if !ok {
		return nil
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
	binding := fmt.Sprintf("%s := strings.%s(%s)\n%s", name, fnName, strings.Join(args, ", "), indent)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("hoist strings.%s out of the loop", fnName),
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
