package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140AuthenticResidualFormalUses(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"helper", "returned", "retained", "wrongReceiver"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetalRewrite(t, "before", func(fs *token.FileSet, files []*ast.File) []*ast.File {
				changed := 0
				for _, file := range files {
					for _, declaration := range file.Decls {
						function, ok := declaration.(*ast.FuncDecl)
						if !ok || function.Recv == nil {
							continue
						}
						if kind == "wrongReceiver" && function.Name.Name == "recordOProj" {
							ast.Inspect(function.Body, func(node ast.Node) bool {
								call, ok := node.(*ast.CallExpr)
								if !ok {
									return true
								}
								method, ok := call.Fun.(*ast.SelectorExpr)
								if !ok || method.Sel.Name != "recordAdd" {
									return true
								}
								receiver, ok := method.X.(*ast.SelectorExpr)
								if ok && receiver.Sel.Name == "wo" {
									receiver.Sel.Name = "wD"
									changed++
								}
								return true
							})
							continue
						}
						receiver, ok := function.Recv.List[0].Type.(*ast.Ident)
						if !ok || receiver.Name != "f32Linear" || function.Name.Name != "recordAdd" || kind == "wrongReceiver" {
							continue
						}
						function.Type.Params.List[1].Names[1].Name = "scratch"
						statement := "observeResidualScratch(scratch)"
						if kind == "returned" {
							statement = "return residualScratchError{scratch}"
						}
						if kind == "retained" {
							statement = "recordedResidualScratch=scratch"
						}
						parsed, err := parser.ParseFile(fs, "residual-use.go", `package llamagpu;var recordedResidualScratch buffer;type residualScratchError struct{value buffer};func(residualScratchError)Error()string{return "scratch"};func observeResidualScratch(buffer){};func injection(){`+statement+`}`, parser.SkipObjectResolution)
						if err != nil {
							t.Fatal(err)
						}
						injection := parsed.Decls[len(parsed.Decls)-1].(*ast.FuncDecl)
						function.Body.List = append(injection.Body.List, function.Body.List...)
						file.Decls = append(file.Decls, parsed.Decls[:len(parsed.Decls)-1]...)
						changed++
					}
				}
				if changed != 1 {
					t.Fatalf("authentic source changes=%d want1", changed)
				}
				return files
			})
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			ownerType := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			concrete := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
			entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == pkg.Func("newDecoder") {
					constructor = ps6136Call(entry, call)
				}
			}
			if constructor == nil {
				t.Fatal("genuine constructor missing")
			}
			var appended ps6125SSAReference
			for _, call := range ps6125ContextCalls(constructor) {
				if candidate := ps6140AppendedBlock(constructor, call, blockType); candidate.value != nil {
					if appended.value != nil {
						t.Fatal("ambiguous append")
					}
					appended = candidate
				}
			}
			if appended.value == nil {
				t.Fatal("genuine append missing")
			}
			object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "encodeStep")
			function := pkg.Prog.FuncValue(object.(*types.Func))
			root := ps6125NewSSAContext(function, nil, nil, 16384)
			owner := root.reference(function.Params[0])
			checked, remaining := 0, 16384
			if !ps6136WalkConsumerCalls(root, func(current *ps6125SSAContext, call *ssa.Call) bool {
				if current.flow.function.Name() != "recordOProj" || !call.Call.IsInvoke() || call.Call.Method.Name() != "recordAdd" {
					return false
				}
				checked++
				proof := ps6140UnusedProjectionFormal(current, call, appended, concrete, owner, ownerType, blockType, ps6136FieldVar(ownerType, "blocks"), ps6136FieldVar(blockType, "wo"), ps6136FieldVar(ownerType, "ao"), ps6136FieldVar(pkg.Pkg.Scope().Lookup("bufSlot").Type(), "b"), 2, 256)
				if proof != nil {
					t.Fatal("mutated exact source formal/receiver accepted")
				}
				return true
			}, &remaining) || checked != 2 {
				t.Fatalf("actual invocation coverage=%d", checked)
			}
		})
	}
}
