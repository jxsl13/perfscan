package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6136AuthenticProfileRecorderBinding(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"original", "wrong_factory", "reversed_error", "wrong_native_field", "wrong_metadata", "late_factory", "wrong_restore", "late_frame"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, "before", true, true)
			if err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			var declaration *ast.FuncDecl
			for _, file := range fixture.files {
				for _, decl := range file.Decls {
					if function, ok := decl.(*ast.FuncDecl); ok && function.Name.Name == "ProfileMetalStep" {
						declaration = function
					}
				}
			}
			if declaration == nil {
				t.Fatal("authentic profile method missing")
			}
			function := fixture.info.Defs[declaration.Name].(*types.Func)
			capacity := function.Type().(*types.Signature).Params().At(2)
			var metadata types.Object
			var literal *ast.FuncLit
			ast.Inspect(declaration.Body, func(node ast.Node) bool {
				if value, ok := node.(*ast.ValueSpec); ok && len(value.Names) == 1 && value.Names[0].Name == "profile" {
					metadata = fixture.info.Defs[value.Names[0]]
				}
				if assignment, ok := node.(*ast.AssignStmt); ok && len(assignment.Rhs) == 1 {
					if candidate, ok := assignment.Rhs[0].(*ast.FuncLit); ok {
						literal = candidate
					}
				}
				return true
			})
			if literal == nil || metadata == nil {
				t.Fatal("actual profile callback flow missing")
			}
			wrapper := fixture.pkg.Scope().Lookup("mProfileRec").Type().(*types.Named)
			base := fixture.pkg.Scope().Lookup("mRec").Type().(*types.Named)
			structure := wrapper.Underlying().(*types.Struct)
			field := base.Underlying().(*types.Struct).Field(0)
			id := "github.com/jxsl13/goai/backend/metal.NewProfilingRecorder"
			switch mutation {
			case "wrong_factory":
				id = "github.com/jxsl13/goai/backend/metal.NewRecorder"
			case "reversed_error":
				literal.Body.List[1].(*ast.IfStmt).Cond.(*ast.BinaryExpr).Op = token.EQL
			case "wrong_native_field":
				field = types.NewVar(token.NoPos, fixture.pkg, "other", field.Type())
			case "wrong_metadata":
				metadata = types.NewVar(token.NoPos, fixture.pkg, "other", metadata.Type())
			case "late_factory":
				literal.Body.List = append(literal.Body.List[:2], &ast.ExprStmt{X: literal.Body.List[0].(*ast.AssignStmt).Rhs[0]}, literal.Body.List[2])
			case "wrong_restore":
				restore := declaration.Body.List[6].(*ast.DeferStmt).Call.Fun.(*ast.FuncLit)
				restore.Body.List[0].(*ast.AssignStmt).Rhs[0] = restore.Body.List[1].(*ast.AssignStmt).Rhs[0]
			case "late_frame":
				declaration.Body.List = append(declaration.Body.List[:13], &ast.ExprStmt{X: literal.Body.List[0].(*ast.AssignStmt).Rhs[0]}, declaration.Body.List[13])
			}
			factoryValid := mutation == "original" || mutation == "wrong_restore" || mutation == "late_frame"
			if got := ps6136ProfileRecorderFactory(pass, literal, id, capacity, metadata, wrapper, base, structure.Field(0), structure.Field(1), field); got != factoryValid {
				t.Fatalf("native/base/metadata identity proof %v", got)
			}
			owner := fixture.pkg.Scope().Lookup("Decoder").Type()
			lookup := func(typ types.Type, name string) *types.Var {
				object, _, _ := types.LookupFieldOrMethod(typ, true, fixture.pkg, name)
				return object.(*types.Var)
			}
			ops := lookup(owner, "ops")
			proof := ps6136ProfileOverrideFrame(pass, declaration, ops, lookup(ops.Type(), "newRecorder"), lookup(ops.Type(), "newDecodeRecorder"), lookup(ops.Type(), "asyncEncode"), "github.com/jxsl13/goai/llamagpu.Decoder.Step", "github.com/jxsl13/goai/llamagpu.Decoder.dropPending", func(callback *ast.FuncLit, actualCapacity, actualMetadata types.Object) bool {
				if mutation == "wrong_metadata" {
					actualMetadata = metadata
				}
				return ps6136ProfileRecorderFactory(pass, callback, id, actualCapacity, actualMetadata, wrapper, base, structure.Field(0), structure.Field(1), field)
			})
			if (proof != nil) != (mutation == "original") {
				t.Fatalf("exact deferred override/restore frame proof %v", proof != nil)
			}
			var methods []*types.Func
			for _, name := range []string{"MatMul", "AddBias"} {
				object, _, _ := types.LookupFieldOrMethod(base, false, fixture.pkg, name)
				methods = append(methods, object.(*types.Func))
			}
			if !ps6136SameAdapterMethods(wrapper, base, methods) {
				t.Fatal("profiling wrapper does not inherit exactly the proved native adapters")
			}
		})
	}
}
