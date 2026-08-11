package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2104 reports a map declared with no size hint immediately before a
// bounded loop that inserts into it. Same bound semantics as PS2101:
// len(src) is exact for one unconditional insert of distinct keys per
// iteration, an upper bound otherwise — either way a valid make() hint.
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
over a slice or map, or a counted loop), make(map[K]V, bound) reserves the
buckets once.

Bound semantics: the hint is exact for one unconditional insert of distinct
keys per iteration and an upper bound for filtered inserts or repeated
keys — a size hint may legally over-reserve, so both are safe.

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
			for i := 0; i+1 < len(block.List); i++ {
				name, typ, ok := unsizedMapDecl(block.List[i])
				if !ok {
					continue
				}
				loop := block.List[i+1]
				body := loopBodyOf(loop)
				if body == nil || !loopInsertsInto(body, name) {
					continue
				}
				capExpr := loopCapacityExpr(pass, loop)
				// A range loop is always bounded by its source; a for
				// loop only counts when a bound was derived.
				if _, isRange := loop.(*ast.RangeStmt); !isRange && capExpr == "" {
					continue
				}
				diag := analysis.Diagnostic{
					Pos:     block.List[i].Pos(),
					End:     block.List[i].End(),
					Message: fmt.Sprintf("%s is filled in the following bounded loop but declared without a size hint; pre-size it with make(map[...]..., bound) — exact for distinct unconditional inserts, an upper bound otherwise", name),
				}
				if capExpr != "" {
					var b printerBuf
					_ = printer.Fprint(&b, token.NewFileSet(), typ)
					newDecl := fmt.Sprintf("%s := make(%s, %s)", name, b.String(), capExpr)
					diag.SuggestedFixes = []analysis.SuggestedFix{{
						Message: fmt.Sprintf("pre-size %s to %s", name, capExpr),
						TextEdits: []analysis.TextEdit{
							{Pos: block.List[i].Pos(), End: block.List[i].End(), NewText: []byte(newDecl)},
						},
					}}
				}
				pass.Report(diag)
			}
			return true
		})
	}
	return nil, nil
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

// loopInsertsInto reports whether body assigns to name[...] in its own
// iteration scope. Nested loops and closures are not descended into: an
// insert there is not bounded by THIS loop's trip count.
func loopInsertsInto(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
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
				found = true
				return false
			}
		}
		return true
	})
	return found
}
