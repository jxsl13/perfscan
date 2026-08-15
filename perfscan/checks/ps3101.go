package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3101 reports loop-invariant string↔[]byte conversions inside loops.
var PS3101 = register(&lint.Check{
	ID:       "PS3101",
	Category: "indirect",
	Slug:     "invariant-conversion-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a loop-invariant string↔[]byte conversion inside a loop",
		Text: `[]byte(s) and string(b) copy their operand. When the operand
does not change across iterations, the same bytes are copied every pass —
hoist the conversion above the loop.

Only conversions of plain identifiers that the loop body does not reassign
are reported, so every finding is a safe hoist.

The automatic fix binds the conversion to a variable immediately before the
OUTERMOST enclosing loop and replaces the call with that variable. It is only
attached when the hoist is provably behavior-preserving: the operand is not
an iteration variable of any enclosing loop; is not assigned, redeclared, or
address-taken anywhere in the outermost loop STATEMENT (a for-loop's
init/cond/post included); is a constant or a variable declared inside the
enclosing function whose address is never taken there and that no function
literal assigns (so no callee can change it between iterations); and the
conversion is spelled with the canonical []byte/string type. A []byte result
is a fresh mutable slice per iteration, so it is only shared across
iterations when it is consumed directly by a read-only stdlib bytes.*
predicate (Contains, Equal, HasPrefix, ... — resolved by type, not by
spelling). string(b) with a slice operand is never hoisted: the loop may
mutate b's elements and a snapshot would freeze stale bytes. Everything else
keeps the plain advisory report.`,
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
			if body == nil || reassignsInLoop(loop, arg.Name) || isLoopVar(loop, arg.Name) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: convType + "(" + arg.Name + ") copies its operand on every iteration but " + arg.Name + " is loop-invariant; hoist the conversion above the loop",
			}
			if fix := hoistConvFix(pass, stack, call, arg); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
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

// bytesReadOnlyFuncs are package-level bytes functions that return only a
// bool or an int: they never write through their arguments and never alias
// them into their results, so a []byte argument shared across iterations is
// observationally identical to a fresh copy.
var bytesReadOnlyFuncs = map[string]bool{
	"Compare":       true,
	"Contains":      true,
	"ContainsAny":   true,
	"ContainsFunc":  true,
	"ContainsRune":  true,
	"Count":         true,
	"Equal":         true,
	"EqualFold":     true,
	"HasPrefix":     true,
	"HasSuffix":     true,
	"Index":         true,
	"IndexAny":      true,
	"IndexByte":     true,
	"IndexFunc":     true,
	"IndexRune":     true,
	"LastIndex":     true,
	"LastIndexAny":  true,
	"LastIndexByte": true,
	"LastIndexFunc": true,
}

// hoistConvFix builds the two-edit hoist for an invariant conversion: insert
// a binding immediately before the OUTERMOST enclosing loop and replace the
// in-loop conversion with the bound variable. It returns nil whenever the
// hoist is not provably behavior-preserving; the caller then reports the
// plain advisory diagnostic.
func hoistConvFix(pass *analysis.Pass, stack []ast.Node, call *ast.CallExpr, arg *ast.Ident) *analysis.SuggestedFix {
	// Only the canonical []byte / string spellings are reproduced in the
	// binding; a named conversion type could be shadowed or carry methods.
	convText, ok := canonicalConvText(pass, call.Fun)
	if !ok {
		return nil
	}
	// A string result is immutable, so sharing one across iterations is
	// always safe — PROVIDED the operand's bytes cannot change between the
	// hoist point and the original site. A string operand is immutable; a
	// SLICE operand (string(b) with b []byte) is not: b[i]++ inside the
	// loop mutates what the conversion reads, and a hoisted snapshot would
	// freeze the pre-mutation state (regression: go/doc/comment's inc()).
	// A []byte result is a fresh mutable slice per iteration: share it
	// only when it is consumed directly by a read-only bytes.* predicate,
	// where neither a write nor an alias can escape.
	if convText == "[]byte" {
		if len(stack) == 0 || !byteConvReadOnly(pass, stack[len(stack)-1], call) {
			return nil
		}
	}
	if convText == "string" {
		if t := pass.TypesInfo.TypeOf(arg); t != nil {
			if _, isSlice := t.Underlying().(*types.Slice); isSlice {
				return nil // mutable operand: hoisting can freeze stale bytes
			}
		}
	}
	outer, ok := astutil.OutermostLoop(stack)
	if !ok {
		return nil
	}
	// The operand must be invariant relative to EVERY enclosing loop up to
	// the outermost, not just the innermost one the report was gated on:
	// it must not be an iteration variable of any of them...
	start := -1
	for i, n := range stack {
		if n == outer {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	// A labeled loop's Pos is after the label: inserting the binding there
	// would re-target the label at the binding and break `break/continue
	// label`.
	if start > 0 {
		if _, labeled := stack[start-1].(*ast.LabeledStmt); labeled {
			return nil
		}
	}
	for _, n := range stack[start:] {
		if astutil.IsLoop(n) && isLoopVar(n, arg.Name) {
			return nil
		}
	}
	// ...and must not be assigned, redeclared, or address-taken anywhere in
	// the outermost loop STATEMENT. Scanning the whole statement — not just
	// its body — also covers a ForStmt's init/cond/post: `for i := 0; ...;
	// i, sep = i+1, next` reassigns sep on every iteration even though its
	// body never mentions it (regression: post-statement mutation).
	if mutatesOrShadows(outer, arg.Name) {
		return nil
	}
	// The loop's own statement is still not enough: the operand can change
	// between iterations through a closure or a pointer even when the loop
	// text never assigns it — `g := func() { sep = "z" }` called from the
	// body, `p := &sep` passed to a mutating callee, or a package-level
	// operand reassigned by any function the loop calls. Require the operand
	// to be a variable declared inside the enclosing function whose address
	// is never taken there and that no function literal in it assigns; a
	// constant is immutable and always safe (regressions: closure-, pointer-,
	// and package-level-mediated mutation).
	switch obj := pass.TypesInfo.ObjectOf(arg).(type) {
	case *types.Const:
		// immutable: nothing can change it between iterations
	case *types.Var:
		if obj.IsField() {
			return nil
		}
		fn := enclosingFuncNode(stack, start)
		if fn == nil || obj.Pos() < fn.Pos() || obj.Pos() >= fn.End() {
			return nil // free variable or package-level: callees may mutate it
		}
		if addressTakenOrClosureMutated(pass, fn, obj) {
			return nil
		}
	default:
		return nil
	}
	pos := pass.Fset.Position(call.Pos())
	name := fmt.Sprintf("psConv%d_%d", pos.Line, pos.Column)
	loopPos := pass.Fset.Position(outer.Pos())
	// Assume gofmt indentation (tabs): the loop starts at column
	// loopPos.Column, i.e. loopPos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", loopPos.Column-1)
	binding := name + " := " + convText + "(" + arg.Name + ")\n" + indent
	return &analysis.SuggestedFix{
		Message: "hoist " + convText + "(" + arg.Name + ") above the loop",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.Pos(), NewText: []byte(binding)},
			{Pos: call.Pos(), End: call.End(), NewText: []byte(name)},
		},
	}
}

// canonicalConvText reports whether the conversion callee is spelled exactly
// as the predeclared []byte or string type, returning its rendered text.
func canonicalConvText(pass *analysis.Pass, fun ast.Expr) (string, bool) {
	switch f := fun.(type) {
	case *ast.ArrayType:
		if f.Len != nil {
			return "", false
		}
		elt, ok := f.Elt.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[elt] != types.Universe.Lookup("byte") {
			return "", false
		}
		return "[]byte", true
	case *ast.Ident:
		if pass.TypesInfo.Uses[f] != types.Universe.Lookup("string") {
			return "", false
		}
		return "string", true
	}
	return "", false
}

// byteConvReadOnly reports whether conv is a direct argument of a read-only
// bytes.* predicate call. The qualifier must RESOLVE (via go/types) to the
// stdlib bytes package, not merely be spelled "bytes": a package-level
// `var bytes ...` declared in ANOTHER file leaves the parser's Ident.Obj nil,
// so a purely syntactic check would accept a method call whose implementation
// may mutate or retain the argument (regression: cross-file bytes shadow).
func byteConvReadOnly(pass *analysis.Pass, parent ast.Node, conv *ast.CallExpr) bool {
	pc, ok := parent.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := pc.Fun.(*ast.SelectorExpr)
	if !ok || !bytesReadOnlyFuncs[sel.Sel.Name] {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	pn, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
	if !ok || pn.Imported().Path() != "bytes" {
		return false
	}
	for _, a := range pc.Args {
		if a == ast.Expr(conv) {
			return true
		}
	}
	return false
}

// reassignsInLoop reports whether the loop STATEMENT — a ForStmt's
// init/cond/post included, not just the body — directly assigns to or
// increments name.
func reassignsInLoop(loop ast.Node, name string) bool {
	found := false
	ast.Inspect(loop, func(n ast.Node) bool {
		switch st := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range st.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := st.X.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		}
		return !found
	})
	return found
}

// enclosingFuncNode returns the innermost function declaration or literal on
// the stack strictly above index i, or nil.
func enclosingFuncNode(stack []ast.Node, i int) ast.Node {
	for j := i - 1; j >= 0; j-- {
		switch stack[j].(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return stack[j]
		}
	}
	return nil
}

// addressTakenOrClosureMutated reports whether, anywhere in fn, the address
// of v is taken or a function literal assigns/increments/range-binds v.
// Either lets an opaque call — from the loop body, cond, or post — change v
// between iterations, which a hoisted binding would not observe.
func addressTakenOrClosureMutated(pass *analysis.Pass, fn ast.Node, v *types.Var) bool {
	usesV := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && pass.TypesInfo.Uses[id] == types.Object(v)
	}
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if ue, ok := n.(*ast.UnaryExpr); ok && ue.Op == token.AND && usesV(ue.X) {
			found = true
		}
		return !found
	})
	if found {
		return true
	}
	ast.Inspect(fn, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return !found
		}
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			switch st := m.(type) {
			case *ast.AssignStmt:
				for _, lhs := range st.Lhs {
					if usesV(lhs) {
						found = true
					}
				}
			case *ast.IncDecStmt:
				if usesV(st.X) {
					found = true
				}
			case *ast.RangeStmt:
				if st.Key != nil && usesV(st.Key) {
					found = true
				}
				if st.Value != nil && usesV(st.Value) {
					found = true
				}
			}
			return !found
		})
		return false // lit.Body already scanned, nested literals included
	})
	return found
}

// mutatesOrShadows reports whether node assigns to, increments, redeclares,
// range-binds, or takes the address of name anywhere within, including
// inside nested closures.
func mutatesOrShadows(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch st := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range st.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := st.X.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		case *ast.RangeStmt:
			if id, ok := st.Key.(*ast.Ident); ok && id.Name == name {
				found = true
			}
			if id, ok := st.Value.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		case *ast.UnaryExpr:
			if st.Op == token.AND {
				if id, ok := st.X.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		case *ast.GenDecl:
			for _, spec := range st.Specs {
				switch sp := spec.(type) {
				case *ast.ValueSpec:
					for _, n2 := range sp.Names {
						if n2.Name == name {
							found = true
						}
					}
				case *ast.TypeSpec:
					if sp.Name.Name == name {
						found = true
					}
				}
			}
		}
		return !found
	})
	return found
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
