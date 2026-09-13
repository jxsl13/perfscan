package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136SharedHelperUnknownSecondInvocation(t *testing.T) {
	t.Parallel()
	fixture, err := ps6136CompileOwnersRewrite(t, "before", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
		changed := 0
		for _, file := range files {
			ast.Inspect(file, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "recordLogits" {
						call.Args = append(call.Args, &ast.Ident{Name: "true"})
					}
				}
				return true
			})
		}
		for _, file := range files {
			for _, declaration := range file.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if function.Name.Name == "recordLogits" {
					function.Type.Params.List = append(function.Type.Params.List, &ast.Field{Names: []*ast.Ident{{Name: "useKnown"}}, Type: &ast.Ident{Name: "bool"}})
					ast.Inspect(function.Body, func(node ast.Node) bool {
						call, ok := node.(*ast.CallExpr)
						if !ok {
							return true
						}
						selector, ok := call.Fun.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						if selector.Sel.Name == "record" {
							selector.X = &ast.Ident{Name: "projector"}
						}
						// Keep the bias extent independently bounded so rejection
						// must come from the unproved projection binding, not an
						// unrelated unknown AddBias extent.
						if selector.Sel.Name == "AddBias" {
							call.Args[3] = &ast.BasicLit{Kind: token.INT, Value: "1"}
						}
						return true
					})
					prefix, err := parser.ParseFile(fs, "unknown-projector-adversary.go", "package llamagpu;func temporary(){projector:=d.out;if !useKnown{projector=unknownProjector()}}", parser.SkipObjectResolution)
					if err != nil {
						t.Fatal(err)
					}
					function.Body.List = append(prefix.Decls[0].(*ast.FuncDecl).Body.List, function.Body.List...)
				}
				if function.Name.Name != "encodeStep" {
					continue
				}
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					selector, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "recordLogits" {
						return true
					}
					second, err := parser.ParseExpr("d.recordLogits(r,unknownRows(),unknownFlag())")
					if err != nil {
						t.Fatal(err)
					}
					original := *call
					call.Fun = &ast.Ident{Name: "firstErr", NamePos: selector.Pos()}
					call.Args = []ast.Expr{&original, second}
					changed++
					return false
				})
			}
		}
		if changed != 1 {
			t.Fatalf("authentic encodeStep projection mutations %d", changed)
		}
		extra, err := parser.ParseFile(fs, "unknown-rows-adversary.go", "package llamagpu;var unknownRowsValue int;func unknownRows()int{return unknownRowsValue};var unknownProjectorValue linear;func unknownProjector()linear{return unknownProjectorValue};var unknownFlagValue bool;func unknownFlag()bool{return unknownFlagValue}", parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return append(files, extra)
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
	field := func(name string) *types.Var {
		object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, name)
		return object.(*types.Var)
	}
	selection := &ps6136Selection{ownerType: owner, workspace: field("logits"), width: field("v"), projector: field("out"), slot: pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)}
	selection.ops = field("ops")
	selection.primaryRecorder = ps6136FieldVar(selection.ops.Type(), "newRecorder")
	selection.secondaryRecorder = ps6136FieldVar(selection.ops.Type(), "newDecodeRecorder")
	step, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Step")
	context := ps6125NewSSAContext(pkg.Prog.FuncValue(step.(*types.Func)), nil, nil, 16384)
	if selection.consumerCalls(context, ps6136OwnerLeaves(pkg), false, nil) != nil {
		t.Fatal("one proved invocation masked the same helper's unknown-row invocation")
	}
	var pool ps6125ExtentPool
	width := ps6125SymbolicExtent(pool.identity(context.flow.function.Params[0].Object(), []*types.Var{selection.width}, false))
	facts := &ps6136Extents{owner: context.reference(context.flow.function.Params[0]), ownerType: owner, immutable: map[*types.Var]ps6125Extent{selection.width: width}, remaining: 16384}
	valid, invalid := 0, 0
	var instruction *ssa.Call
	remaining := 16384
	if !ps6136WalkConsumerCalls(context, func(current *ps6125SSAContext, call *ssa.Call) bool {
		if current.flow.function.Name() != "recordLogits" || !call.Call.IsInvoke() || call.Call.Method.Name() != "record" {
			return false
		}
		if instruction != nil && instruction != call {
			t.Fatal("counterexample did not share one SSA leaf instruction")
		}
		instruction = call
		if _, ok := ps6136LeafExtent(current, call, &ps6136OwnerLeaves(pkg)[0], facts, selection.workspace, selection.slot, selection.projector, selection.width); ok {
			valid++
		} else {
			invalid++
		}
		return true
	}, &remaining) {
		t.Fatal("counterexample traversal exhausted")
	}
	if valid < 1 || invalid < 1 || valid != invalid {
		t.Fatalf("same-instruction invocation bindings valid=%d invalid=%d", valid, invalid)
	}
}
