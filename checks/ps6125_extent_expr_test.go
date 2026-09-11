package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6125ExtentSourceExpressions(t *testing.T) {
	t.Parallel()
	const source = `package extent
type header struct { width int }
type decoder struct { header; other header; rows int; tokens []int }
func opaque() int { return 7 }
func geometry(d, sibling *decoder, tokens []int, narrow uint32, rows int) {
 retained := d.rows * d.width
 common := 1 * d.header.width
 reversed := d.header.width * d.rows
 bulk := len(tokens) * d.width
 siblingWidth := sibling.width
 otherWidth := d.other.width
 fieldLength := len(d.tokens)
 variable := rows
 folded := 4 * (1 << 10)
 unknownCall := opaque()
 unknownAdd := rows + 1
 unknownSlice := len(tokens[:1])
 unknownNarrow := int(narrow)
 unknownUnsigned := narrow * 2
 unknownNegative := -1
 unknownCap := cap(tokens)
 unknownConvert := int(rows)
 _ = []int{retained, common, reversed, bulk, siblingWidth, otherWidth, fieldLength, variable, folded, unknownCall, unknownAdd, unknownSlice, unknownNarrow, unknownNegative, unknownCap, unknownConvert}
 _ = unknownUnsigned
}
func shadowed(len func([]int) int, tokens []int) { unknownShadow := len(tokens); _ = unknownShadow }
`
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "extent.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("extent", files, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	var pool ps6125ExtentPool
	resolve := func(root types.Object, fields []*types.Var, length bool) ps6125Extent {
		return ps6125SymbolicExtent(pool.identity(root, fields, length))
	}
	// This test supplies symbolic point facts explicitly. It tests lowering,
	// not a proof that all these variables or receiver paths stay unchanged.
	values := make(map[string]ps6125Extent)
	expressions := make(map[string]ast.Expr)
	ast.Inspect(file, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		name := assignment.Lhs[0].(*ast.Ident).Name
		expressions[name] = assignment.Rhs[0]
		values[name] = ps6125ExtentExpression(pass, assignment.Rhs[0], resolve)
		return true
	})
	for _, name := range []string{"retained", "common", "reversed", "bulk", "siblingWidth", "otherWidth", "fieldLength", "variable", "folded"} {
		if !values[name].known {
			t.Fatalf("supported source expression %s is unresolved", name)
		}
	}
	if !ps6125SameExtent(values["retained"], values["reversed"]) {
		t.Fatal("promoted and explicit field paths disagree")
	}
	for _, name := range []string{"siblingWidth", "otherWidth", "fieldLength", "variable"} {
		if ps6125SameExtent(values["common"], values[name]) {
			t.Fatalf("distinct typed path %s was conflated with decoder width", name)
		}
	}
	if !ps6125SameExtent(values["folded"], ps6125ConstantExtent(4096)) {
		t.Fatal("typed compile-time constant was not preserved")
	}
	for _, name := range []string{"unknownCall", "unknownAdd", "unknownSlice", "unknownNarrow", "unknownUnsigned", "unknownNegative", "unknownCap", "unknownConvert", "unknownShadow"} {
		if values[name].known {
			t.Fatalf("unsupported expression %s became a geometry proof", name)
		}
	}
	if ps6125ExtentExpression(pass, expressions["retained"], nil).known {
		t.Fatal("missing point facts became symbolic source evidence")
	}
	unknown := func(types.Object, []*types.Var, bool) ps6125Extent { return ps6125Extent{} }
	if ps6125ExtentExpression(pass, expressions["bulk"], unknown).known {
		t.Fatal("unresolved source binding was ignored")
	}
}
