package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3007 reports a membership set (map[K]bool / map[K]struct{}) built by
// ranging a slice and then probed inside a loop.
var PS3007 = register(&lint.Check{
	ID:       "PS3007",
	Category: "indirect",
	Slug:     "set-map-from-slice",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "a membership set built from a slice the caller already owns, probed in a loop",
		Text: `When a set's contents come from a slice the code already holds,
the fix is often no map at all: scanning the source slice directly beats the
map build plus one hash per probe as long as the source stays small (the
goai reference measured the crossover at 8–16 elements on an M2 Pro). This
is a SMALL-SET transform — large sets should keep the map.

Two narrowings keep the check honest, both inherited from the reference:
the set must be read-only after its build loop (a mutable working set
genuinely needs a map), and a build already guarded by a size threshold on
the source is silent — that code has taken this advice and kept the map as
its large-set fallback. An emptiness guard (len(src) > 0) is not a
threshold.

Hotness is not visible to the AST: confirm the source is small and the
probe repeats, then benchmark.`,
		Before: `breakers := make(map[int64]bool, len(seq))
for _, b := range seq {
	breakers[b] = true
}
for _, tok := range window {
	if breakers[tok] { ... }
}`,
		After: `for _, tok := range window {
	if slices.Contains(seq, tok) { ... } // seq is small
}`,
		MeasuredWin: "goai reference: nlp applyDRY -18.72% (19.52µs→15.87µs, p=0.002) with runtime.mapaccess1_fast64 leaving the profile entirely",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3007",
		Doc:  "membership set built from an owned slice, probed in a loop",
		Run:  runPS3007,
	},
})

func runPS3007(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkSetMapFunc(pass, fn)
		}
	}
	return nil, nil
}

func isSetMapType(t types.Type) bool {
	m, ok := t.Underlying().(*types.Map)
	if !ok {
		return false
	}
	switch v := m.Elem().Underlying().(type) {
	case *types.Basic:
		return v.Kind() == types.Bool
	case *types.Struct:
		return v.NumFields() == 0
	}
	return false
}

func checkSetMapFunc(pass *analysis.Pass, fn *ast.FuncDecl) {
	// Builds of the shape `for _, v := range SRC { SET[v] = ... }` with a
	// single-statement body, where SET has a set-shaped map type.
	type build struct {
		loop *ast.RangeStmt
		src  ast.Expr
	}
	builds := map[string]build{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok || r.Value == nil || r.Body == nil || len(r.Body.List) != 1 {
			return true
		}
		v, ok := r.Value.(*ast.Ident)
		if !ok {
			return true
		}
		as, ok := r.Body.List[0].(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 {
			return true
		}
		idx, ok := as.Lhs[0].(*ast.IndexExpr)
		if !ok {
			return true
		}
		m, ok := idx.X.(*ast.Ident)
		if !ok {
			return true
		}
		k, ok := idx.Index.(*ast.Ident)
		if !ok || k.Name != v.Name {
			return true
		}
		t := pass.TypesInfo.TypeOf(idx.X)
		if t == nil || !isSetMapType(t) {
			return true
		}
		// The range source must be slice-typed (an owned scan target).
		if st := pass.TypesInfo.TypeOf(r.X); st == nil {
			return true
		} else if _, ok := st.Underlying().(*types.Slice); !ok {
			return true
		}
		builds[m.Name] = build{loop: r, src: r.X}
		return true
	})
	if len(builds) == 0 {
		return
	}

	inBuild := func(name string, n ast.Node) bool {
		b, ok := builds[name]
		return ok && n.Pos() >= b.loop.Pos() && n.End() <= b.loop.End()
	}

	// Narrowing 1: drop sets written outside their build loop.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, l := range as.Lhs {
			ix, ok := l.(*ast.IndexExpr)
			if !ok {
				continue
			}
			m, ok := ix.X.(*ast.Ident)
			if !ok {
				continue
			}
			if _, tracked := builds[m.Name]; tracked && !inBuild(m.Name, ix) {
				delete(builds, m.Name)
			}
		}
		return true
	})

	// Narrowing 2: drop builds guarded by a size threshold on the source
	// (already-taken advice). Emptiness guards (compare against 0) do not
	// count.
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		for name, b := range builds {
			if b.loop != r {
				continue
			}
			for _, anc := range stack {
				ifs, ok := anc.(*ast.IfStmt)
				if ok && condHasSizeThreshold(ifs.Cond) {
					delete(builds, name)
					break
				}
			}
		}
		return true
	})
	if len(builds) == 0 {
		return
	}

	// Probes: reads of the set inside a loop, outside the build.
	reported := map[string]bool{}
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		idx, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		m, ok := idx.X.(*ast.Ident)
		if !ok || reported[m.Name] {
			return true
		}
		b, tracked := builds[m.Name]
		if !tracked || inBuild(m.Name, idx) {
			return true
		}
		// Skip assignment targets (writes were already handled, but a
		// comma-ok read must be distinguished from a write target).
		if len(stack) > 0 {
			if as, ok := stack[len(stack)-1].(*ast.AssignStmt); ok && as.Tok == token.ASSIGN {
				if slices.Contains(as.Lhs, ast.Expr(idx)) {
					return true
				}
			}
		}
		if _, inLoop := astutil.InLoop(stack); !inLoop {
			return true
		}
		reported[m.Name] = true
		src := exprString(b.src)
		pass.Report(analysis.Diagnostic{
			Pos:     idx.Pos(),
			End:     idx.End(),
			Message: fmt.Sprintf("set %s is built from slice %s and only probed; for a small source, scanning %s directly beats the map build plus a hash per probe (crossover ≈8–16 elements)", m.Name, src, src),
		})
		return true
	})
}

// condHasSizeThreshold reports whether cond compares a len(...) against a
// nonzero integer literal.
func condHasSizeThreshold(cond ast.Expr) bool {
	found := false
	ast.Inspect(cond, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch be.Op {
		case token.LSS, token.LEQ, token.GTR, token.GEQ:
		default:
			return true
		}
		isLen := func(e ast.Expr) bool {
			c, ok := e.(*ast.CallExpr)
			return ok && astutil.CalleeName(c.Fun) == "len"
		}
		nonzeroLit := func(e ast.Expr) bool {
			l, ok := e.(*ast.BasicLit)
			if !ok || l.Kind != token.INT {
				return false
			}
			v, err := strconv.Atoi(l.Value)
			return err == nil && v != 0
		}
		if (isLen(be.X) && nonzeroLit(be.Y)) || (isLen(be.Y) && nonzeroLit(be.X)) {
			found = true
			return false
		}
		return true
	})
	return found
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	}
	return "the source"
}
