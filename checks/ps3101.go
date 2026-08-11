package checks

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3101 reports loop-invariant string↔[]byte conversions inside loops.
var PS3101 = register(&lint.Check{
	ID:       "PS3101",
	Category: "indirect",
	Slug:     "invariant-conversion-in-loop",
	Level:    lint.LevelIdiomatic,
	Doc: lint.Documentation{
		Title: "a loop-invariant string↔[]byte conversion inside a loop",
		Text: `[]byte(s) and string(b) copy their operand. When the operand
does not change across iterations, the same bytes are copied every pass —
hoist the conversion above the loop.

Only conversions of plain identifiers that the loop body does not reassign
are reported, so every finding is a safe hoist.`,
		Before: `for _, line := range lines {
	if bytes.Contains(line, []byte(sep)) { ... }
}`,
		After: `bsep := []byte(sep)
for _, line := range lines {
	if bytes.Contains(line, bsep) { ... }
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3101",
		Doc:  "loop-invariant string/[]byte conversion in a loop",
		Run:  runPS3101,
	},
})

func runPS3101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			arg, ok := call.Args[0].(*ast.Ident)
			if !ok {
				return true
			}
			convType, isConv := conversionType(pass, call)
			if !isConv {
				return true
			}
			loop, inLoop := astutil.InLoop(stack)
			if !inLoop {
				return true
			}
			body := astutil.LoopBody(loop)
			if body == nil || reassigns(body, arg.Name) || isLoopVar(loop, arg.Name) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("%s(%s) copies its operand on every iteration but %s is loop-invariant; hoist the conversion above the loop", convType, arg.Name, arg.Name),
			})
			return true
		})
	}
	return nil, nil
}

// conversionType reports whether call is a string↔[]byte conversion,
// returning its rendered target type.
func conversionType(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	// The callee must denote a type, not a function.
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return "", false
	}
	argT := pass.TypesInfo.TypeOf(call.Args[0])
	if argT == nil {
		return "", false
	}
	dst := tv.Type.Underlying()
	src := argT.Underlying()
	isStr := func(t types.Type) bool {
		b, ok := t.(*types.Basic)
		return ok && b.Info()&types.IsString != 0
	}
	isBytes := func(t types.Type) bool {
		s, ok := t.(*types.Slice)
		if !ok {
			return false
		}
		b, ok := s.Elem().Underlying().(*types.Basic)
		return ok && b.Kind() == types.Byte
	}
	switch {
	case isBytes(dst) && isStr(src):
		return "[]byte", true
	case isStr(dst) && isBytes(src):
		return "string", true
	}
	return "", false
}

// isLoopVar reports whether name is an iteration variable of loop.
func isLoopVar(loop ast.Node, name string) bool {
	switch l := loop.(type) {
	case *ast.RangeStmt:
		if id, ok := l.Key.(*ast.Ident); ok && id.Name == name {
			return true
		}
		if id, ok := l.Value.(*ast.Ident); ok && id.Name == name {
			return true
		}
	case *ast.ForStmt:
		if as, ok := l.Init.(*ast.AssignStmt); ok {
			for _, lhs := range as.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					return true
				}
			}
		}
	}
	return false
}
