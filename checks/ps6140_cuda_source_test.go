package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// Actual provider entries select each model class. The original owner files
// stay intact; in particular neither the MoE path nor the post/sandwich-norm
// paths are synthesized by changing flags on a dense constructor.
func TestPS6140AuthenticCUDASourceClasses(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, targetOS := range []string{"linux", "windows"} {
			for _, test := range []struct {
				name, model, entry, factory, concrete string
				ao, mo                                bool
			}{
				{"dense", "Llama", "NewCUDA", "newDecoder", "f32Linear", true, true},
				{"moe", "Mixtral", "NewMixtralCUDA", "newMixtralDecoder", "f32Linear", true, false},
				{"postnorm", "OLMo2", "NewOLMo2CUDA", "newOLMo2Decoder", "f32Linear", false, false},
				{"sandwich", "Gemma2", "NewGemma2CUDA", "newGemma2Decoder", "f32Linear", false, false},
				{"quantized", "QuantLlama", "NewQuantCUDA", "newQuantDecoder", "quantLinear", false, false},
			} {
				t.Run(revision+"/"+targetOS+"/"+test.name, func(t *testing.T) {
					t.Parallel()
					fixture, err := ps6140CompileLoadedCUDA(t, revision, targetOS)
					if err != nil {
						t.Fatal(err)
					}
					pkg := ps6136FixtureSSA(fixture)
					pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
					contract := ps6140AuthenticSourceContract(revision, "ao")
					contract.ModelType = "github.com/jxsl13/goai/nlp." + test.model
					contract.ConstructorEntry = "github.com/jxsl13/goai/llamagpu." + test.entry
					contract.ConstructorFunction = "github.com/jxsl13/goai/llamagpu." + test.factory
					contract.BackendAllocator = "github.com/jxsl13/goai/backend/cuda.NewDeviceBufferF32"
					contract.NativeReleaseMethod = "github.com/jxsl13/goai/backend/cuda.DeviceF32.Release"
					contract.NativeAllocationCountUnit = "float32-elements"
					for index := range contract.Projections {
						contract.Projections[index].ConcreteType = "github.com/jxsl13/goai/llamagpu." + test.concrete
					}
					if test.name == "moe" {
						// There is no dense block.wD in this constructor: the actual
						// nested experts consume mo, whereas attention still ignores ao.
						contract.Projections = contract.Projections[:1]
					}
					selected := ps6136ContractsContext(pass).unusedProjectionSelection(pkg, &contract, 262144)
					if selected == nil || selected.entry.flow.function != pkg.Func(test.entry) || selected.allocation.context.flow.function != pkg.Func(test.factory) {
						t.Fatal("actual CUDA constructor binding prerequisite missing")
					}
					s := selected.allocation
					collection := ps6140SelectedCollectionCheck(pass, pkg, s.context, s.ownerType, selected.block, selected.blocks, selected.projectors, 262144, func(stage string) { t.Log(stage) })
					if collection == nil || collection.owner != s.owner {
						t.Fatal("actual CUDA projection collection prerequisite missing")
					}
					snapshot := ps6140ConstructorFlags(s.context, s.owner, s.ownerType, selected.flags, 262144)
					flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, s.ownerType, 262144)
					if flags == nil {
						t.Fatal("actual CUDA architecture flags prerequisite missing")
					}
					for _, member := range []string{"ao", "mo"} {
						want := test.ao
						if member == "mo" {
							want = test.mo
						}
						workspace := ps6136FieldVar(s.ownerType, member)
						slot := ps6136FieldVar(workspace.Type(), "b")
						for _, method := range []string{"encodeStep", "stepN"} {
							object, _, _ := types.LookupFieldOrMethod(s.ownerType, true, pkg.Pkg, method)
							scenario := ps6140NewConstructorScenario(collection, flags, s.ownerType, pkg.Prog.FuncValue(object.(*types.Func)), 262144)
							if scenario == nil {
								t.Fatalf("actual %s scenario prerequisite missing", method)
							}
							uses := ps6140ScenarioWorkspaceUses(scenario, workspace, slot, 2, 262144)
							if (uses != nil) != want {
								t.Errorf("%s/%s unused source closure=%v want=%v", method, member, uses != nil, want)
							}
						}
						contract.WorkspaceField = member
						if revision == "before" {
							actual := ps6136ContractsContext(pass).unusedProjectionSelection(pkg, &contract, 262144)
							if actual == nil || ps6140ConstructorAllocation(actual.allocation, 262144) == nil {
								t.Fatalf("%s genuine eager allocation prerequisite missing", member)
							}
						}
						proof := ps6140UnusedProjectionSource(pass, pkg, &contract, 262144)
						if (proof != nil) != (revision == "before" && want) {
							t.Logf("source initialization=%v eager-allocation=%v", s.initializationValues(262144), ps6140ConstructorAllocation(s, 262144) != nil)
							t.Errorf("%s source assembly=%v want=%v", member, proof != nil, revision == "before" && want)
						}
						if proof != nil && (len(proof.retained.public.entries) != 9 || proof.retained.public.publication != proof.selection.entry || ps6090FunctionID(proof.nativeRelease) != contract.NativeReleaseMethod) {
							t.Fatal("complete CUDA public owner inventory or actual native release binding lost")
						}
					}
				})
			}
		}
	}
}
