package checks

import (
	"go/ast"
	"go/printer"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2104 reports a map declared with no size hint earlier in the block
// of a bounded loop that inserts into it. Same counted bound semantics as
// PS2101: k unconditional inserts per iteration make k*bound the hint
// (exact for distinct keys), conditional inserts are excluded (lower
// bound), a conditional-only fill keeps 1*bound (upper bound).
var PS2104 = register(&lint.Check{
	ID:       "PS2104",
	Category: "alloc",
	Slug:     "map-without-prealloc",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a map filled in a bounded loop directly after a declaration with no size hint",
		Text: `A map declared without a size hint starts at minimum capacity
and rehashes its way up as the loop inserts — each growth re-buckets every
key inserted so far. When the fill loop's iteration count is known (a range
over a slice or map, or a counted loop) and nothing between the
declaration and the loop touches the map, make(map[K]V, k*bound) reserves
the buckets once.

Bound semantics — inserts are COUNTED per iteration: k unconditional
inserts make k*bound the hint (exact for distinct keys); conditional
inserts are excluded, leaving a lower bound; a conditional-only fill keeps
1*bound, the upper bound. A size hint may legally over-reserve, so every
class is safe.

The automatic fix rewrites the declaration when the bound is a plain
identifier (or len of one) not reassigned in the loop body. A size hint is
not observable through map semantics, so the rewrite cannot change
behavior.`,
		Before: `index := map[string]int{}
for i, s := range src {
	index[s] = i
}`,
		After: `index := make(map[string]int, len(src))
for i, s := range src {
	index[s] = i
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2104",
		Doc:  "map filled in a bounded loop declared without a size hint",
		Run:  runPS2104,
	},
})

func runPS2104(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for j, stmt := range block.List {
				body := loopBodyOf(stmt)
				if body == nil {
					continue
				}
				bound := loopCapacityExpr(pass, stmt)
				// A range loop is always bounded by its source; a for
				// loop only counts when a bound was derived.
				if _, isRange := stmt.(*ast.RangeStmt); !isRange && bound == "" {
					continue
				}
				reportMapPrealloc(pass, block, j, bound)
			}
			return true
		})
	}
	return nil, nil
}

// reportMapPrealloc pairs every earlier unsized map declaration in the
// block with the loop at index j, as long as nothing between them touches
// the variable.
func reportMapPrealloc(pass *analysis.Pass, block *ast.BlockStmt, j int, bound string) {
	body := loopBodyOf(block.List[j])
	for i := 0; i < j; i++ {
		name, typ, ok := unsizedMapDecl(block.List[i])
		if !ok || stmtsMention(block.List[i+1:j], name) {
			continue
		}
		uncond, cond := insertCounts(body, name)
		if uncond == 0 && cond == 0 {
			continue
		}
		boundWord := bound
		if boundWord == "" {
			boundWord = "bound"
		}
		capExpr, class := capacityForCounts(boundWord, uncond, cond, false)
		diag := analysis.Diagnostic{
			Pos:     block.List[i].Pos(),
			End:     block.List[i].End(),
			Message: name + " is filled in the following bounded loop but declared without a size hint; pre-size it with make(map[...]..., " + capExpr + ") — " + class + " (a hint may over-reserve for repeated keys)",
		}
		if bound != "" {
			var b printerBuf
			_ = printer.Fprint(&b, token.NewFileSet(), typ)
			newDecl := name + " := make(" + b.String() + ", " + capExpr + ")"
			diag.SuggestedFixes = []analysis.SuggestedFix{{
				Message: "pre-size " + name + " to " + capExpr,
				TextEdits: []analysis.TextEdit{
					{Pos: block.List[i].Pos(), End: block.List[i].End(), NewText: []byte(newDecl)},
				},
			}}
		}
		pass.Report(diag)
	}
}

type printerBuf struct{ b []byte }

func (p *printerBuf) Write(bs []byte) (int, error) {
	p.b = append(p.b, bs...)
	return len(bs), nil
}
func (p *printerBuf) String() string { return string(p.b) }

// unsizedMapDecl matches `m := map[K]V{}` (empty literal) and
// `m := make(map[K]V)` (no size hint), returning the name and map type.
func unsizedMapDecl(s ast.Stmt) (string, ast.Expr, bool) {
	as, ok := s.(*ast.AssignStmt)
	if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return "", nil, false
	}
	id, ok := as.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	switch rhs := as.Rhs[0].(type) {
	case *ast.CompositeLit:
		if mt, ok := rhs.Type.(*ast.MapType); ok && len(rhs.Elts) == 0 {
			return id.Name, mt, true
		}
	case *ast.CallExpr:
		if fn, ok := rhs.Fun.(*ast.Ident); ok && fn.Name == "make" && len(rhs.Args) == 1 {
			if mt, ok := rhs.Args[0].(*ast.MapType); ok {
				return id.Name, mt, true
			}
		}
	}
	return "", nil, false
}

// insertCounts counts assignments to name[...] per iteration of body,
// separated into unconditional (top-level statements) and conditional
// (under an if/switch/select) inserts. Nested loops and closures are not
// descended into: an insert there is not bounded by THIS loop's trip
// count. A multi-assign statement counts each name[...] target.
func insertCounts(body *ast.BlockStmt, name string) (uncond, cond int) {
	condDepth := 0
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
			return false
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			condDepth++
			defer func() { condDepth-- }()
			for _, c := range childrenOf(n) {
				ast.Inspect(c, walk)
			}
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			ix, ok := lhs.(*ast.IndexExpr)
			if !ok {
				continue
			}
			if id, ok := ix.X.(*ast.Ident); ok && id.Name == name {
				if condDepth > 0 {
					cond++
				} else {
					uncond++
				}
			}
		}
		return true
	}
	ast.Inspect(body, walk)
	return uncond, cond
}
