package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Exercise the allocation component against the actual eager owner. This is
// not a deadness test: architecture/use closure is a separate required join.
func TestPS6140AuthenticConstructorAllocation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		revision, member, name, extra string
		earlyRelease                  bool
	}{
		{revision: "before", member: "ao"}, {revision: "before", member: "mo"}, {revision: "after", member: "ao"}, {revision: "after", member: "mo"},
		{"before", "ao", "retained-length", "func(d *Decoder) RetainedObserver()int{return len(d.all)}", false},
		{"before", "ao", "retained-capacity", "func(d *Decoder) RetainedObserver()int{return cap(d.all)}", false},
		{"before", "ao", "retained-global-alias", "var observedBuffers []buffer;func(d *Decoder) RetainedObserver(){observedBuffers=d.all}", false},
		{"before", "ao", "retained-return-alias", "func(d *Decoder) RetainedObserver()[]buffer{return d.all}", false},
		{"before", "ao", "retained-index-release", "func(d *Decoder) RetainedObserver(){d.all[0].Release()}", false},
		{"before", "ao", "retained-clear", "func(d *Decoder) RetainedObserver(){d.all=nil}", false},
		{"before", "ao", "retained-append", "func(d *Decoder) RetainedObserver(){d.all=append(d.all,nil)}", false},
		{revision: "before", member: "ao", name: "release-before-success", earlyRelease: true},
	} {
		name := test.revision + "/" + test.member
		if test.name != "" {
			name += "/" + test.name
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			member := test.member
			fixture, err := ps6140CompileLoadedMetalRewrite(t, test.revision, func(fs *token.FileSet, files []*ast.File) []*ast.File {
				if test.earlyRelease {
					inserted := 0
					for _, file := range files {
						for _, declaration := range file.Decls {
							function, ok := declaration.(*ast.FuncDecl)
							if !ok || function.Name.Name != "newDecoder" {
								continue
							}
							last := len(function.Body.List) - 1
							returned, ok := function.Body.List[last].(*ast.ReturnStmt)
							if !ok || len(returned.Results) != 2 {
								t.Fatal("authentic successful-return mutation target missing")
							}
							name, ok := returned.Results[0].(*ast.Ident)
							if !ok || name.Name != "d" {
								t.Fatal("authentic returned owner changed")
							}
							// Keep the genuine final aggregate-error barrier intact.
							// The new phase proof must reject release before that
							// barrier, not borrow its already-closed return block.
							guard, ok := function.Body.List[last-1].(*ast.IfStmt)
							if !ok {
								t.Fatal("authentic final error guard missing")
							}
							call := &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "d", NamePos: guard.Pos()}, Sel: &ast.Ident{Name: "Release", NamePos: guard.Pos()}}, Lparen: guard.Pos(), Rparen: guard.Pos()}}
							function.Body.List = append(function.Body.List[:last-1], call, guard, returned)
							inserted++
						}
					}
					if inserted != 1 {
						t.Fatalf("early-release mutations=%d want=1", inserted)
					}
				}
				if test.extra == "" {
					return files
				}
				file, err := parser.ParseFile(fs, "retained_observer.go", "package llamagpu\n"+test.extra, parser.SkipObjectResolution)
				if err != nil {
					t.Fatal(err)
				}
				return append(files, file)
			})
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			function := pkg.Func("newDecoder")
			entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var context *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == function {
					context = ps6136Call(entry, call)
				}
			}
			if context == nil {
				t.Fatal("actual selected constructor missing")
			}
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			model := function.Params[0].Type().(*types.Pointer).Elem().(*types.Named)
			selection := &ps6136Selection{
				context: context, ownerType: owner, modelType: model,
				model:       context.reference(function.Params[0]),
				maximumRows: ps6136FieldVar(owner, "maxLen"), width: ps6136FieldVar(owner, "d"),
				modelConfig: ps6136FieldVar(model, "Config"), workspace: ps6136FieldVar(owner, member),
				ops: ps6136FieldVar(owner, "ops"), retained: ps6136FieldVar(owner, "all"),
				allocatorIDs: map[string]bool{"github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32": true},
			}
			selection.configRows = ps6136FieldVar(selection.modelConfig.Type(), "Ctx")
			selection.configWidth = ps6136FieldVar(selection.modelConfig.Type(), "Dim")
			selection.slot = ps6136FieldVar(selection.workspace.Type(), "b")
			allocator, index, _ := types.LookupFieldOrMethod(selection.ops.Type(), true, pkg.Pkg, "newBuffer")
			selection.allocator, selection.allocatorIndex = allocator.(*types.Var), index[0]
			allocationMethod := "allocScratch"
			if test.revision == "after" {
				allocationMethod = "allocResidualScratch"
			}
			allocate, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, allocationMethod)
			release, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Release")
			selection.allocationFunction = ps6090FunctionID(allocate.(*types.Func))
			selection.releaseMethod = ps6090FunctionID(release.(*types.Func))
			bufferRelease, _, _ := types.LookupFieldOrMethod(selection.slot.Type(), true, pkg.Pkg, "Release")
			selection.retainedReleaseMethod = ps6090FunctionID(bufferRelease.(*types.Func))
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					returned, ok := instruction.(*ssa.Return)
					if !ok || len(returned.Results) == 0 {
						continue
					}
					if value, ok := returned.Results[0].(*ssa.Const); ok && value.IsNil() {
						continue
					}
					selection.owner = ps6136OwnerRoot(context, returned.Results[0], owner)
				}
			}
			proof := ps6140ConstructorAllocation(selection, 16384)
			if test.revision == "before" && (proof == nil || proof.allocation == nil || proof.factory == nil || proof.errorCell.value == nil) {
				t.Fatal("actual context-by-dimension factory/retention/error flow rejected")
			}
			if test.revision == "after" && proof != nil {
				t.Fatal("actual conditional/placeholder allocation treated as unconditional eager storage")
			}
			wrong := *selection
			wrong.configWidth = ps6136FieldVar(selection.modelConfig.Type(), "Vocab")
			if ps6140ConstructorAllocation(&wrong, 16384) != nil {
				t.Fatal("unrelated output-head width certified residual allocation")
			}
			wrong = *selection
			wrong.allocatorIDs = map[string]bool{"unrelated.NewBuffer": true}
			if ps6140ConstructorAllocation(&wrong, 16384) != nil || ps6140ConstructorAllocation(selection, 0) != nil || ps6140ConstructorAllocation(nil, 16384) != nil {
				t.Fatal("wrong native allocator, missing selection or exhausted budget admitted")
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			concrete := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
			collection := ps6140SelectedCollection(pass, pkg, context, owner, block, ps6136FieldVar(owner, "blocks"), map[*types.Var]*types.Named{ps6136FieldVar(block, "wo"): concrete, ps6136FieldVar(block, "wD"): concrete}, 65536)
			if collection == nil || collection.owner != selection.owner {
				t.Fatal("actual selected allocation/collection identity missing")
			}
			fields := make(map[*types.Var]bool)
			for _, name := range []string{"postNorm", "sandwich", "moe", "mla", "mamba", "mamba2", "rwkv", "jamba"} {
				fields[ps6136FieldVar(owner, name)] = true
			}
			snapshot := ps6140ConstructorFlags(context, collection.owner, owner, fields, 65536)
			flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 131072)
			if flags == nil {
				t.Fatal("actual immutable constructor class prerequisite missing")
			}
			if !ps6140ConstructorCallbacksCheck(context, owner, selection.workspace, 262144, func(stage string) { t.Log(stage) }) {
				t.Fatal("constructor callback closure missing")
			}
			if ps6140PublicConstructorScope(context, flags, owner, 262144) != entry {
				ps6140ConstructorFlagsCheck(entry, collection.owner, owner, fields, 262144, func(stage string) { t.Log(stage) })
				t.Fatal("public constructor publication scope missing")
			}
			if !ps6140ConstructorCallbacksCheck(entry, owner, selection.workspace, 262144, func(stage string) { t.Log(stage) }) {
				t.Fatal("public constructor callback closure missing")
			}
			if test.revision == "before" && !ps6140ConstructorStorageCheck(entry, selection.owner, owner, selection.workspace, proof, 262144, func(stage string) { t.Log(stage) }) {
				t.Fatal("public constructor storage closure missing")
			}
			joined := ps6140RetainedPublicSource(pkg, selection, collection, flags, 2, 262144)
			entryCount := 10
			if test.extra != "" {
				entryCount++
			}
			if test.revision == "before" && (joined == nil || joined.owner != selection.owner || joined.constructor != context || joined.workspace != selection.workspace || len(joined.entries) != entryCount) {
				t.Fatal("actual allocation did not join all ten public source entries")
			}
			if test.revision == "before" {
				retained := ps6140RetainedListSource(pass, pkg, selection, joined, 262144)
				if (retained != nil) != (test.extra == "") {
					ps6140RetainedListSourceCheck(pass, pkg, selection, joined, 262144, func(stage string) { t.Log(stage) })
					t.Fatalf("actual retained-list source closure=%v want=%v", retained != nil, test.extra == "")
				}
				if retained != nil && (retained.retentions[proof.factory] == nil || retained.public != joined) {
					t.Fatal("selected retention invocation identity lost")
				}
				if retained != nil {
					if ps6140NativeReleaseBinding(joined.storage, retained.elementClose, "github.com/jxsl13/goai/backend/metal.DeviceBuffer.Release", 262144) == nil {
						t.Fatal("actual retained element did not dispatch to its exact native allocator-result pointer")
					}
					if !selection.constructorLifetime(joined.allocation, 262144) {
						t.Fatal("genuine allocation-failure cleanup prerequisite missing")
					}
					if got := ps6140ConstructorReleasePhase(joined.publication, joined.owner, owner, retained.release, 262144); got != !test.earlyRelease {
						t.Fatalf("actual successful-publication release phase=%v want=%v", got, !test.earlyRelease)
					}
					if got := ps6140RetainedLifetimeSource(selection, retained, 262144); got != !test.earlyRelease {
						t.Fatalf("actual joined source lifetime=%v want=%v", got, !test.earlyRelease)
					}
					if ps6140RetainedLifetimeSource(selection, retained, 0) || ps6140RetainedLifetimeSource(selection, nil, 262144) {
						t.Fatal("missing retained certificate or exhausted source-lifetime budget admitted")
					}
				}
			}
			if test.revision == "after" {
				// Still prove all actual public unused-formal paths. Quietness
				// must come from the changed allocation, not broken dispatch.
				uses := ps6140PublicWorkspaceUses(pkg, collection, flags, owner, selection.workspace, selection.slot, 2, 262144)
				if joined != nil || len(uses) != 10 {
					t.Fatal("after source allocation boundary or genuine use prerequisites changed")
				}
			}
			wrong = *selection
			wrong.context = ps6125NewSSAContext(function, nil, nil, 16384)
			if ps6140RetainedPublicSource(pkg, &wrong, collection, flags, 2, 262144) != nil || ps6140RetainedPublicSource(pkg, selection, collection, flags, 2, 0) != nil {
				t.Fatal("unrelated constructor invocation or exhausted budget joined")
			}
			// The configured assembler must retain every independent gate:
			// a valid vocabulary cannot erase a loaded observer or early release.
			contract := ps6140AuthenticSourceContract(test.revision, member)
			assembled := ps6140UnusedProjectionSource(pass, pkg, &contract, 262144)
			wantAssembled := test.revision == "before" && test.extra == "" && !test.earlyRelease
			if (assembled != nil) != wantAssembled {
				t.Fatalf("configured source conjunction=%v want=%v", assembled != nil, wantAssembled)
			}
		})
	}
}
