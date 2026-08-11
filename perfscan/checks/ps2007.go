package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2007 reports a call given the same size-like argument twice whose square
// result is then indexed at exactly that position.
var PS2007 = register(&lint.Check{
	ID:       "PS2007",
	Category: "alloc",
	Slug:     "build-nxn-use-one-row",
	Level:    lint.LevelAggressive,
	Doc: lint.Documentation{
		Title: "an N×N object materialized to consume one row",
		Text: `A call given the SAME size expression twice builds a square
result whose cost grows with the argument in both dimensions; when the
consumer then indexes it at exactly that position, one row was needed and
N were built — and on a per-token path, an order higher again over the run.

The give-away is the REPEATED argument. The driving position must be fixed,
not a loop bound: when it bounds a loop the square result is walked in full
(an attention mask genuinely consumed whole is not this pattern).

L3 (aggressive): the fix computes the needed row from whatever the callee
derives it from — usually exposing the per-element rule rather than its
materialized matrix. Check bit-identity: a value delivered through a
one-hot matmul equals the gathered entry for finite tables, but NOT for
±Inf/NaN (0·Inf is NaN) nor a stored -0 (0 + -0 = +0).`,
		Before: `full, _ := d.relBias.Bias(ctx, pos+1, pos+1) // builds [pos+1, pos+1, heads]
fs := full.Storage().F64()
v := fs[(pos*kk+k)*heads+h] // reads row pos, discards the rest`,
		After:       `v := d.relBias.BiasRow(ctx, pos, k, h) // per-element rule, no matrix`,
		MeasuredWin: "reference corpus: T5 relative-position bias 130x at pos=32, 713x at pos=512; allocation 86.8MB→19.7KB per call; end-to-end decode 4.82x and 13.87GB→125MB",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2007",
		Doc:  "square build consumed at one row",
		Run:  runPS2007,
	},
})

func runPS2007(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkBuildNxN(pass, fn)
		}
	}
	return nil, nil
}

func checkBuildNxN(pass *analysis.Pass, fn *ast.FuncDecl) {
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		// The last two arguments must be the SAME non-trivial expression.
		a, b := call.Args[len(call.Args)-2], call.Args[len(call.Args)-1]
		if !exprEqual(a, b) {
			return true
		}
		base := sizeDrivingIdent(a)
		if base == "" {
			return true
		}
		// A position that bounds a loop means the square result is walked
		// in full — not this pattern.
		if identIsLoopBound(fn.Body, base) {
			return true
		}
		lhs, ok := assignedIdent(as)
		if !ok || !derivedValueIndexedBy(fn.Body, lhs, base) {
			return true
		}
		pass.Report(analysis.Diagnostic{
			Pos:     as.Pos(),
			End:     as.End(),
			Message: fmt.Sprintf("%s is called with %s twice, building a square result, and %s then indexes it — an N×N object materialized to consume one row; compute the row directly from the callee's per-element rule (verify bit-identity for ±Inf/NaN/-0)", callName(call), exprTextRendered(a), base),
		})
		return true
	})
}

func callName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return "the callee"
}

// sizeDrivingIdent returns the identifier a size expression is built from —
// pos for pos, pos+1, pos*2 — or "" when none drives it.
func sizeDrivingIdent(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.ParenExpr:
		return sizeDrivingIdent(x.X)
	case *ast.BinaryExpr:
		if l := sizeDrivingIdent(x.X); l != "" {
			return l
		}
		return sizeDrivingIdent(x.Y)
	}
	return ""
}

// identIsLoopBound reports whether name appears in any loop condition or
// range source in body.
func identIsLoopBound(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch l := n.(type) {
		case *ast.ForStmt:
			if l.Cond != nil && exprMentions(l.Cond, name) {
				found = true
			}
		case *ast.RangeStmt:
			if exprMentions(l.X, name) {
				found = true
			}
		}
		return !found
	})
	return found
}

// assignedIdent returns the first non-blank, non-err identifier an
// assignment binds.
func assignedIdent(as *ast.AssignStmt) (string, bool) {
	for _, lhs := range as.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" && id.Name != "err" {
			return id.Name, true
		}
	}
	return "", false
}

// derivedValueIndexedBy reports whether root — or any value transitively
// derived from it by assignment — is indexed by an expression mentioning
// pos. Following the derivation chain is what makes this precise: a site
// may read the matrix as fs := full.Storage().F64() and index fs, never
// full.
func derivedValueIndexedBy(body *ast.BlockStmt, root, pos string) bool {
	derived := map[string]bool{root: true}
	// Two passes so a chain assigned in order (a := f(root); b := g(a)) is
	// followed.
	for range 2 {
		ast.Inspect(body, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, rhs := range as.Rhs {
				if !mentionsAnyOf(rhs, derived) {
					continue
				}
				if name, ok := assignedIdent(as); ok {
					derived[name] = true
				}
			}
			return true
		})
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		ix, ok := n.(*ast.IndexExpr)
		if !ok {
			return true
		}
		if derived[baseIdentName(ix.X)] && exprMentions(ix.Index, pos) {
			found = true
		}
		return !found
	})
	return found
}

func mentionsAnyOf(e ast.Expr, names map[string]bool) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && names[id.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}
