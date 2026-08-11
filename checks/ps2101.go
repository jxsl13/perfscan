package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2101 reports a slice declared with no capacity immediately before a
// loop that appends to it, when the loop's iteration source bounds the
// element count. The bound is EXACT for one unconditional append per
// iteration, an UPPER bound for filtered appends (the same worst case
// append itself reserves), and a LOWER bound when the body appends more
// than once per iteration — a floor is still worth taking.
//
// PS2101 is a perfscan-original check (the x1xx block per category is
// reserved for checks that did not originate in the goai reference
// registry).
var PS2101 = register(&lint.Check{
	ID:       "PS2101",
	Category: "alloc",
	Slug:     "append-without-prealloc",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a slice built by append in a bounded loop directly after an unsized declaration",
		Text: `Appending into a slice declared without capacity grows the
backing array geometrically — each growth allocates and copies everything
appended so far. When the append target is declared immediately before a
loop whose iteration count is known — a range over a slice or map, or a
counted for loop — that count bounds the appends, so
make([]T, 0, bound) removes every growth copy.

Bound semantics: for one unconditional append per iteration the bound is
exact; for a filtered append it is an upper bound (the same worst case
append itself reserves); for multiple appends per iteration it is a lower
bound and still removes the early growth copies.

The automatic fix rewrites the declaration when the bound is a plain
identifier (or len of one) that the loop body does not reassign. Capacity
is not observable through append semantics, so the rewrite is
bit-identical for the built slice's contents.`,
		Before: `out := []string{}
for _, s := range src {
	out = append(out, s)
}`,
		After: `out := make([]string, 0, len(src))
for _, s := range src {
	out = append(out, s)
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2101",
		Doc:  "append into unsized slice declared directly before a bounded loop",
		Run:  runPS2101,
	},
})

func runPS2101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i+1 < len(block.List); i++ {
				name, typ, ok := unsizedSliceDecl(block.List[i])
				if !ok {
					continue
				}
				loop := block.List[i+1]
				body := loopBodyOf(loop)
				if body == nil || !loopAppendsTo(body, name) {
					continue
				}
				capExpr := loopCapacityExpr(pass, loop)
				// A range loop is always bounded by its source; a for
				// loop only counts when a bound was derived.
				if _, isRange := loop.(*ast.RangeStmt); !isRange && capExpr == "" {
					continue
				}
				reportPrealloc(pass, block.List[i], name, typ, capExpr)
			}
			return true
		})
	}
	return nil, nil
}

func loopBodyOf(s ast.Stmt) *ast.BlockStmt {
	switch l := s.(type) {
	case *ast.RangeStmt:
		return l.Body
	case *ast.ForStmt:
		return l.Body
	}
	return nil
}

// loopCapacityExpr derives the capacity expression that bounds the loop's
// iteration count, or "" when none is safely derivable.
//
//   - `for … := range src` with src a plain identifier of slice, array or
//     map type → "len(src)"
//   - `for i := 0; i < n; i++` with n a plain identifier → "n"
//   - `for i := 0; i < len(src); i++` with src a plain identifier → "len(src)"
func loopCapacityExpr(pass *analysis.Pass, s ast.Stmt) string {
	switch l := s.(type) {
	case *ast.RangeStmt:
		src := simpleExprText(l.X)
		if src == "" || reassigns(l.Body, rootIdentName(l.X)) {
			return ""
		}
		t := pass.TypesInfo.TypeOf(l.X)
		if t == nil {
			return ""
		}
		switch t.Underlying().(type) {
		case *types.Slice, *types.Array, *types.Map:
			return "len(" + src + ")"
		case *types.Pointer: // *[N]T range
			return "len(" + src + ")"
		}
		return ""
	case *ast.ForStmt:
		// i := <lit>; i < bound; i++ (or i += lit)
		init, ok := l.Init.(*ast.AssignStmt)
		if !ok || len(init.Lhs) != 1 {
			return ""
		}
		iv, ok := init.Lhs[0].(*ast.Ident)
		if !ok {
			return ""
		}
		cond, ok := l.Cond.(*ast.BinaryExpr)
		if !ok || (cond.Op != token.LSS && cond.Op != token.LEQ) {
			return ""
		}
		lhs, ok := cond.X.(*ast.Ident)
		if !ok || lhs.Name != iv.Name {
			return ""
		}
		var bound, subject string
		switch b := cond.Y.(type) {
		case *ast.Ident, *ast.SelectorExpr:
			bound = simpleExprText(cond.Y)
			subject = rootIdentName(cond.Y)
		case *ast.CallExpr:
			if len(b.Args) == 1 && calleeIsLen(b) {
				if inner := simpleExprText(b.Args[0]); inner != "" {
					bound = "len(" + inner + ")"
					subject = rootIdentName(b.Args[0])
				}
			}
		}
		if bound == "" {
			return ""
		}
		// The bound's subject must not be reassigned in the body.
		if reassigns(l.Body, subject) {
			return ""
		}
		return bound
	}
	return ""
}

func calleeIsLen(call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	return ok && id.Name == "len"
}

// simpleExprText renders a side-effect-free bound source — a plain
// identifier or a selector chain over one (x, x.f, x.f.g) — and returns ""
// for anything else (calls, index expressions), which cannot be safely
// repeated in a make() capacity.
func simpleExprText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		if base := simpleExprText(x.X); base != "" {
			return base + "." + x.Sel.Name
		}
	}
	return ""
}

// rootIdentName returns the root identifier of a selector chain.
func rootIdentName(e ast.Expr) string {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x.Name
		case *ast.SelectorExpr:
			e = x.X
		default:
			return ""
		}
	}
}

// unsizedSliceDecl matches `s := []T{}`, `s := make([]T, 0)` and
// `var s []T`, returning the variable name and the slice type expression.
func unsizedSliceDecl(s ast.Stmt) (string, ast.Expr, bool) {
	switch st := s.(type) {
	case *ast.AssignStmt:
		if st.Tok != token.DEFINE || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			return "", nil, false
		}
		id, ok := st.Lhs[0].(*ast.Ident)
		if !ok {
			return "", nil, false
		}
		switch rhs := st.Rhs[0].(type) {
		case *ast.CompositeLit:
			if at, ok := rhs.Type.(*ast.ArrayType); ok && at.Len == nil && len(rhs.Elts) == 0 {
				return id.Name, rhs.Type, true
			}
		case *ast.CallExpr:
			if fn, ok := rhs.Fun.(*ast.Ident); ok && fn.Name == "make" && len(rhs.Args) == 2 {
				if at, ok := rhs.Args[0].(*ast.ArrayType); ok && at.Len == nil {
					if lit, ok := rhs.Args[1].(*ast.BasicLit); ok && lit.Value == "0" {
						return id.Name, rhs.Args[0], true
					}
				}
			}
		}
	case *ast.DeclStmt:
		gd, ok := st.Decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR || len(gd.Specs) != 1 {
			return "", nil, false
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 0 {
			return "", nil, false
		}
		if at, ok := vs.Type.(*ast.ArrayType); ok && at.Len == nil {
			return vs.Names[0].Name, vs.Type, true
		}
	}
	return "", nil, false
}

// loopAppendsTo reports whether body contains `name = append(name, ...)` in
// its own iteration scope. Nested loops and closures are not descended
// into: an append there is not bounded by THIS loop's trip count, and the
// bound claim would be wrong.
func loopAppendsTo(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.RangeStmt, *ast.ForStmt, *ast.FuncLit:
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok || lhs.Name != name {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "append" && len(call.Args) > 0 {
			if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func reportPrealloc(pass *analysis.Pass, decl ast.Stmt, name string, typ ast.Expr, capExpr string) {
	diag := analysis.Diagnostic{
		Pos:     decl.Pos(),
		End:     decl.End(),
		Message: fmt.Sprintf("%s is appended to in the following bounded loop but declared without capacity; pre-size it with make(..., 0, bound) — exact for one unconditional append per iteration, an upper bound for filtered appends", name),
	}
	if capExpr != "" { // bound already validated against body reassignment
		var b strings.Builder
		_ = printer.Fprint(&b, token.NewFileSet(), typ)
		newDecl := fmt.Sprintf("%s := make(%s, 0, %s)", name, b.String(), capExpr)
		diag.SuggestedFixes = []analysis.SuggestedFix{{
			Message: fmt.Sprintf("pre-size %s to %s", name, capExpr),
			TextEdits: []analysis.TextEdit{
				{Pos: decl.Pos(), End: decl.End(), NewText: []byte(newDecl)},
			},
		}}
	}
	pass.Report(diag)
}

// reassigns reports whether body assigns to name.
func reassigns(body *ast.BlockStmt, name string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
