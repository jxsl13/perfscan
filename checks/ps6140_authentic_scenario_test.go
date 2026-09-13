package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPS6140AuthenticConstructorScenario(t *testing.T) {
	t.Parallel()
	for _, methodName := range []string{"encodeStep", "stepN"} {
		t.Run(methodName, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, "before")
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			concrete := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
			entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == pkg.Func("newDecoder") {
					constructor = ps6136Call(entry, call)
				}
			}
			if constructor == nil {
				t.Fatal("actual selected public constructor missing")
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			projectors := map[*types.Var]*types.Named{ps6136FieldVar(block, "wo"): concrete, ps6136FieldVar(block, "wD"): concrete}
			collection := ps6140SelectedCollection(pass, pkg, constructor, owner, block, ps6136FieldVar(owner, "blocks"), projectors, 65536)
			if collection == nil || !collection.nilFields[ps6136FieldVar(block, "moeRouter")] {
				t.Fatal("actual selected immutable block/nil router prerequisite missing")
			}
			fields := make(map[*types.Var]bool)
			for _, name := range []string{"postNorm", "sandwich", "moe", "mla", "mamba", "mamba2", "rwkv", "jamba"} {
				fields[ps6136FieldVar(owner, name)] = true
			}
			snapshot := ps6140ConstructorFlags(constructor, collection.owner, owner, fields, 65536)
			flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 131072)
			if flags == nil {
				t.Fatal("source architecture class prerequisite missing")
			}
			object, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, methodName)
			scenario := ps6140NewConstructorScenario(collection, flags, owner, pkg.Prog.FuncValue(object.(*types.Func)), 131072)
			if scenario == nil {
				t.Fatal("authentic conditional method scenario rejected")
			}
			calls := make(map[string]int)
			if !scenario.walk(scenario.root, func(current *ps6125SSAContext, call *ssa.Call) bool {
				if function := call.Call.StaticCallee(); function != nil {
					switch function.Name() {
					case "recordRWKVBlock", "recordMambaBlock", "recordMamba2Block", "recordMLAAttention", "recordMoEFFN":
						t.Fatalf("unselected architecture call remained reachable: %s", function.Name())
					}
				}
				if !call.Call.IsInvoke() || call.Call.Method.Name() != "recordAdd" {
					return false
				}
				var projection, workspace *types.Var
				switch current.flow.function.Name() {
				case "recordOProj":
					projection, workspace = ps6136FieldVar(block, "wo"), ps6136FieldVar(owner, "ao")
				case "recordDownProj":
					projection, workspace = ps6136FieldVar(block, "wD"), ps6136FieldVar(owner, "mo")
				default:
					t.Fatalf("unexpected residual source method %s", current.flow.function.Name())
				}
				proof := ps6140UnusedProjectionFormal(current, call, collection.appended, concrete, scenario.receiver, owner, block, collection.blocks, projection, workspace, ps6136FieldVar(workspace.Type(), "b"), 2, 256)
				if proof == nil {
					t.Fatalf("actual selected %s scratch formal not joined", current.flow.function.Name())
				}
				calls[workspace.Name()]++
				return true
			}) {
				t.Fatal("conditional method traversal exhausted")
			}
			if calls["ao"] != 1 || calls["mo"] != 15 {
				t.Fatalf("actual selected residual method inventory changed: %v", calls)
			}
			t.Logf("selected source unused-formal invocations: %v", calls)
			for _, name := range []string{"ao", "mo"} {
				workspace := ps6136FieldVar(owner, name)
				complete := ps6140NewConstructorScenario(collection, flags, owner, pkg.Prog.FuncValue(object.(*types.Func)), 131072)
				uses := ps6140ScenarioWorkspaceUses(complete, workspace, ps6136FieldVar(workspace.Type(), "b"), 2, 131072)
				if uses == nil {
					t.Fatalf("complete selected method use closure failed for %s", name)
				}
				if uses.scenario != complete || uses.workspace != workspace || uses.reads == 0 || uses.leaves != calls[name] {
					t.Fatalf("conditional workspace association lost for %s", name)
				}
				if methodName == "encodeStep" {
					entries := ps6140PublicOwnerEntries(pkg, owner, 262144)
					if len(entries) != 10 {
						t.Fatalf("complete loaded public owner entries=%d want 10", len(entries))
					}
					for _, entry := range entries {
						public := ps6140NewConstructorScenarioEntry(collection, flags, owner, entry.function, entry.parameter, 262144)
						if ps6140ScenarioWorkspaceClosureCheck(public, workspace, ps6136FieldVar(workspace.Type(), "b"), 2, 262144, func(stage string) { t.Log(stage) }) == nil {
							t.Fatalf("public %s use closure failed for %s", entry.function.Name(), name)
						}
					}
					if len(ps6140PublicWorkspaceUses(pkg, collection, flags, owner, workspace, ps6136FieldVar(workspace.Type(), "b"), 2, 262144)) != len(entries) {
						t.Fatalf("complete public workspace census failed for %s", name)
					}
				}
			}
		})
	}
}
