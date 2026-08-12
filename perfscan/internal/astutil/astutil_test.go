package astutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// parseSnippet parses src (a full Go file) and returns its AST root.
func parseSnippet(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

// callStacks walks root and records, for every call expression f(...) whose
// callee is a bare identifier, the ancestor stack captured at that call. Marker
// calls in the fixtures below (init(), body(), …) are each unique, so the map
// gives a direct handle on "the stack at that syntactic position".
func callStacks(root ast.Node) map[string][]ast.Node {
	out := map[string][]ast.Node{}
	WithStack(root, func(n ast.Node, stack []ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok {
				cp := make([]ast.Node, len(stack))
				copy(cp, stack)
				out[id.Name] = cp
			}
		}
		return true
	})
	return out
}

// TestInLoopAndOutermostLoop pins the two subtle contracts: a node in a loop's
// init/cond/post (or inside a nested function literal) is NOT "in" the loop,
// and InLoop returns the innermost enclosing loop while OutermostLoop returns
// the outermost — both stopping at a function-literal boundary.
func TestInLoopAndOutermostLoop(t *testing.T) {
	src := `package p
func f(n int, xs []int) {
	for i := initt(); condt(i); postt(i) {
		body()
		for range xs {
			inner()
			go func() { closure() }()
		}
	}
	outside()
}`
	stacks := callStacks(parseSnippet(t, src))

	inLoop := map[string]bool{
		"initt":   false, // for-init: not on the iteration path
		"condt":   false, // for-cond
		"postt":   false, // for-post
		"body":    true,  // outer for body
		"inner":   true,  // inner range body
		"closure": false, // inside a func literal (a boundary)
		"outside": false, // not in any loop
	}
	for name, want := range inLoop {
		st, ok := stacks[name]
		if !ok {
			t.Fatalf("fixture missing call %s()", name)
		}
		if _, got := InLoop(st); got != want {
			t.Errorf("InLoop at %s() = %v, want %v", name, got, want)
		}
	}

	// InLoop returns the INNERMOST loop: at inner() it is the range loop, at
	// body() it is the outer for. OutermostLoop returns the outer for for both.
	innerLoop, _ := InLoop(stacks["inner"])
	bodyLoop, _ := InLoop(stacks["body"])
	if innerLoop == bodyLoop {
		t.Error("InLoop at inner() must be the range loop, distinct from the outer for at body()")
	}
	if _, ok := InLoop(stacks["inner"]); !ok {
		t.Fatal("inner() should be in a loop")
	}
	outerAtInner, ok1 := OutermostLoop(stacks["inner"])
	outerAtBody, ok2 := OutermostLoop(stacks["body"])
	if !ok1 || !ok2 || outerAtInner != outerAtBody {
		t.Errorf("OutermostLoop at inner() and body() must both be the outer for")
	}
	if outerAtInner != ast.Node(bodyLoop) {
		t.Error("OutermostLoop at inner() must equal the outer for (the loop enclosing body())")
	}
	// Past a func-literal boundary there is no enclosing loop.
	if _, ok := OutermostLoop(stacks["closure"]); ok {
		t.Error("OutermostLoop must not cross a function-literal boundary")
	}
}

func TestIsLoopAndLoopBody(t *testing.T) {
	src := `package p
func f(xs []int) {
	for i := 0; i < 3; i++ {
	}
	for range xs {
	}
	if true {
	}
}`
	f := parseSnippet(t, src)
	var forStmt, rangeStmt, ifStmt ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.ForStmt:
			forStmt = n
		case *ast.RangeStmt:
			rangeStmt = n
		case *ast.IfStmt:
			ifStmt = n
		}
		return true
	})
	if !IsLoop(forStmt) || !IsLoop(rangeStmt) {
		t.Error("IsLoop must be true for for and range statements")
	}
	if IsLoop(ifStmt) {
		t.Error("IsLoop must be false for an if statement")
	}
	if LoopBody(forStmt) == nil || LoopBody(rangeStmt) == nil {
		t.Error("LoopBody must return the body of for/range")
	}
	if LoopBody(ifStmt) != nil {
		t.Error("LoopBody must be nil for a non-loop")
	}
}

// TestPkgFuncCall covers the qualifier resolution: a real pkg.Fn selector
// matches, a method call on a value does not, a shadowed package name does not,
// and the set filters by function name.
func TestPkgFuncCall(t *testing.T) {
	src := `package p
import "fmt"
func f(w writer) {
	fmt.Println("a")
	fmt.Sprintf("b")
	w.Emit("c")
}
type writer struct{}
func (writer) Emit(string) {}
`
	f := parseSnippet(t, src)
	calls := map[string]*ast.CallExpr{} // keyed by selector name (all distinct here)
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				calls[sel.Sel.Name] = call
			}
		}
		return true
	})

	set := map[string]bool{"Println": true}
	if name, ok := PkgFuncCall(calls["Println"].Fun, "fmt", set); !ok || name != "Println" {
		t.Errorf("PkgFuncCall(fmt.Println) = %q,%v want Println,true", name, ok)
	}
	// Sprintf is a real fmt call but not in the set -> not matched.
	if _, ok := PkgFuncCall(calls["Sprintf"].Fun, "fmt", set); ok {
		t.Error("PkgFuncCall must respect the name set (Sprintf not in it)")
	}
	// A nil set matches any name.
	if _, ok := PkgFuncCall(calls["Sprintf"].Fun, "fmt", nil); !ok {
		t.Error("PkgFuncCall with a nil set must match any fmt function")
	}
	// w.Emit is a method call on a value, not the fmt package qualifier.
	if _, ok := PkgFuncCall(calls["Emit"].Fun, "fmt", nil); ok {
		t.Error("PkgFuncCall must not match a method call on a value (w.Emit)")
	}
	// A local variable named like the package shadows it (Ident.Obj != nil).
	shadow := `package p
func f() {
	fmt := struct{ Println func(string) }{}
	fmt.Println("x")
}`
	sf := parseSnippet(t, shadow)
	var shadowed *ast.CallExpr
	ast.Inspect(sf, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if _, ok := call.Fun.(*ast.SelectorExpr); ok {
				shadowed = call
			}
		}
		return true
	})
	if _, ok := PkgFuncCall(shadowed.Fun, "fmt", nil); ok {
		t.Error("PkgFuncCall must not match when fmt is a shadowing local variable")
	}
}

func TestCalleeName(t *testing.T) {
	src := `package p
func f() {
	bare()
	pkg.Method()
	(func(){})()
}`
	f := parseSnippet(t, src)
	var got []string
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			got = append(got, CalleeName(call.Fun))
		}
		return true
	})
	// bare() -> "bare"; pkg.Method() -> "Method"; a func-literal call -> "".
	want := map[string]bool{"bare": true, "Method": true, "": true}
	for _, g := range got {
		delete(want, g)
	}
	if len(want) != 0 {
		t.Errorf("CalleeName results %v missing %v", got, want)
	}
}
