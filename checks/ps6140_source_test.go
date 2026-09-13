package checks

import (
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func ps6140AuthenticSourceContract(revision, member string) config.UnusedProjectionScratchContract {
	const local = "github.com/jxsl13/goai/llamagpu."
	allocate := "allocScratch"
	if revision == "after" {
		allocate = "allocResidualScratch"
	}
	return config.UnusedProjectionScratchContract{
		OwnerType: local + "Decoder", ModelType: "github.com/jxsl13/goai/nlp.Llama", ConstructorEntry: local + "New", ConstructorFunction: local + "newDecoder",
		ModelConfigField: "Config", ConfigRowsField: "Ctx", ConfigWidthField: "Dim", MaximumRowsField: "maxLen", WidthField: "d",
		WorkspaceField: member, SlotBufferField: "b", RetainedListField: "all", BackendOpsField: "ops", BackendAllocatorField: "newBuffer",
		BackendAllocator: "github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32", AllocationFunction: local + "Decoder." + allocate,
		ReleaseMethod: local + "Decoder.Release", NativeReleaseMethod: "github.com/jxsl13/goai/backend/metal.DeviceBuffer.Release",
		BlockType: local + "block", BlocksField: "blocks", Projections: []config.UnusedProjectionBinding{{Field: "wo", ConcreteType: local + "f32Linear"}, {Field: "wD", ConcreteType: local + "f32Linear"}},
		ClassFlags: []string{"postNorm", "sandwich", "moe", "mla", "mamba", "mamba2", "rwkv", "jamba"}, UnusedFormal: 2,
		NativeAllocationCountBits: 32, NativeAllocationCountUnit: "bytes", NativeSizeBits: 64, ProfileRows: 2048, ProfileWidth: 2048,
		// Test input assertions are not source facts or native execution tests.
		FreshIndependentFloat32StorageReviewed: true, NativeFailureOwnershipReviewed: true, NativeInputCopyAndAliasesReviewed: true,
		NativeReleaseAndFinalizerReviewed: true, SequentialCompletionReviewed: true, PositiveCheckedSourceGeometryReviewed: true,
		NativeByteCountRangeReviewed: true, ExternalOwnerObservationsReviewed: true, AllProviderBuildPathsReviewed: true,
	}
}

func TestPS6140AuthenticSourceBindingAdversaries(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		edit     func(*config.UnusedProjectionScratchContract)
		selected bool
	}{
		{"missing-owner", func(c *config.UnusedProjectionScratchContract) {
			c.OwnerType = "github.com/jxsl13/goai/llamagpu.Missing"
		}, false},
		{"wrong-model", func(c *config.UnusedProjectionScratchContract) { c.ModelType = "github.com/jxsl13/goai/nlp.GPT" }, false},
		{"private-entry", func(c *config.UnusedProjectionScratchContract) { c.ConstructorEntry = c.ConstructorFunction }, false},
		{"missing-factory", func(c *config.UnusedProjectionScratchContract) {
			c.ConstructorFunction = "github.com/jxsl13/goai/llamagpu.Missing"
		}, false},
		{"missing-allocation", func(c *config.UnusedProjectionScratchContract) {
			c.AllocationFunction = "github.com/jxsl13/goai/llamagpu.Decoder.Missing"
		}, false},
		{"wrong-allocation", func(c *config.UnusedProjectionScratchContract) { c.AllocationFunction = c.ReleaseMethod }, true},
		{"wrong-release-signature", func(c *config.UnusedProjectionScratchContract) {
			c.ReleaseMethod = "github.com/jxsl13/goai/llamagpu.Decoder.Vocab"
		}, false},
		{"wrong-list-element", func(c *config.UnusedProjectionScratchContract) { c.RetainedListField = "blocks"; c.BlocksField = "all" }, false},
		{"missing-slot", func(c *config.UnusedProjectionScratchContract) { c.SlotBufferField = "missing" }, false},
		{"wrong-backend-callback", func(c *config.UnusedProjectionScratchContract) { c.BackendAllocatorField = "newRecorder" }, false},
		{"missing-native-allocator", func(c *config.UnusedProjectionScratchContract) {
			c.BackendAllocator = "github.com/jxsl13/goai/backend/metal.Missing"
		}, false},
		{"missing-native-release", func(c *config.UnusedProjectionScratchContract) {
			c.NativeReleaseMethod = "github.com/jxsl13/goai/backend/metal.DeviceBuffer.Missing"
		}, false},
		{"missing-projection", func(c *config.UnusedProjectionScratchContract) { c.Projections[0].Field = "missing" }, false},
		{"wrong-concrete-projection", func(c *config.UnusedProjectionScratchContract) {
			c.Projections[0].ConcreteType = "github.com/jxsl13/goai/llamagpu.quantLinear"
		}, true},
		{"missing-class-flag", func(c *config.UnusedProjectionScratchContract) { c.ClassFlags[0] = "missing" }, false},
		{"integer-is-not-flag", func(c *config.UnusedProjectionScratchContract) { c.ClassFlags[0] = "h" }, false},
		{"wrong-config-width", func(c *config.UnusedProjectionScratchContract) { c.ConfigWidthField = "Vocab" }, true},
		{"used-formal", func(c *config.UnusedProjectionScratchContract) { c.UnusedFormal = 1 }, true},
		{"out-of-range-formal", func(c *config.UnusedProjectionScratchContract) { c.UnusedFormal = 1000 }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, "before")
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			contract := ps6140AuthenticSourceContract("before", "ao")
			if ps6140UnusedProjectionSource(pass, pkg, &contract, 262144) == nil {
				t.Fatal("genuine complete source-assembly prerequisite missing before binding mutation")
			}
			test.edit(&contract)
			if !contract.Valid() {
				t.Fatal("mutation was rejected by vocabulary syntax, not typed source roles")
			}
			selected := ps6136ContractsContext(pass).unusedProjectionSelection(pkg, &contract, 262144)
			if (selected != nil) != test.selected {
				t.Fatalf("typed source selection=%v want=%v", selected != nil, test.selected)
			}
			if ps6140UnusedProjectionSource(pass, pkg, &contract, 262144) != nil {
				t.Fatal("configured identity or reviewed flags substituted for actual source proof")
			}
		})
	}
}

func TestPS6140SelectedConstructorInvocations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"direct", "return build(m)", true},
		{"forwarded", "return forward(m)", true},
		{"passed-factory", "return through(build,m)", true},
		{"returned-factory", "f:=factoryValue();return f(m)", true},
		{"not-invoked", "return &owner{},nil", false},
		{"twice", "_,_=build(m);return build(m)", false},
		{"twice-indirect", "_,_=through(build,m);return through(build,m)", false},
		{"conditional-invocations", "if b{return build(m)};return build(m)", false},
		{"recursive-helper", "return recursive(m)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type model struct{};type owner struct{}
func build(*model)(*owner,error){return &owner{},nil}
func forward(m *model)(*owner,error){return build(m)}
func through(f func(*model)(*owner,error),m *model)(*owner,error){return f(m)}
func factoryValue()func(*model)(*owner,error){return build}
func recursive(m *model)(*owner,error){return recursive(m)}
func New(m *model,b bool)(*owner,error){`+test.body+`}`)
			root := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 256)
			factory := pkg.Func("build")
			if root == nil || factory == nil {
				t.Fatal("genuine source entry/factory missing")
			}
			selected := ps6140SelectedConstructor(root, factory, 256)
			if (selected != nil) != test.want {
				t.Fatalf("selected invocation=%v want=%v", selected != nil, test.want)
			}
			if selected != nil && (selected.flow.function != factory || selected.parent == nil || selected.site == nil || selected.parent.calls[selected.site] != selected) {
				t.Fatal("actual call-context chain lost")
			}
			if ps6140SelectedConstructor(root, factory, 0) != nil || ps6140SelectedConstructor(nil, factory, 256) != nil || ps6140SelectedConstructor(root, (*ssa.Function)(nil), 256) != nil {
				t.Fatal("missing source or zero budget admitted")
			}
		})
	}
}

func TestPS6140AuthenticSourceAssembly(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, member := range []string{"ao", "mo"} {
			t.Run(revision+"/"+member, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6140CompileLoadedMetal(t, revision)
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
				contract := ps6140AuthenticSourceContract(revision, member)
				selection := ps6136ContractsContext(pass).unusedProjectionSelection(pkg, &contract, 262144)
				if selection == nil || selection.allocation.workspace.Name() != member || selection.entry.flow.function != pkg.Func("New") || selection.allocation.context.flow.function != pkg.Func("newDecoder") {
					t.Fatal("actual configured constructor/field identities did not resolve")
				}
				proof := ps6140UnusedProjectionSource(pass, pkg, &contract, 262144)
				if (proof != nil) != (revision == "before") {
					t.Fatalf("assembled source certificate=%v want=%v", proof != nil, revision == "before")
				}
				if proof != nil {
					public := proof.retained.public
					if public.owner != proof.selection.allocation.owner || proof.collection.owner != public.owner || proof.flags.snapshot.owner != public.owner || public.publication != proof.selection.entry || len(public.entries) != 10 || ps6090FunctionID(proof.nativeRelease) != contract.NativeReleaseMethod {
						t.Fatal("assembled source owner/publication/dispatch/release identities lost")
					}
				}
				if revision == "after" {
					// Quietness is the changed eager allocation, not missing roles
					// or a broken configured runtime-use/dispatch prerequisite.
					s := selection.allocation
					collection := ps6140SelectedCollection(pass, pkg, s.context, s.ownerType, selection.block, selection.blocks, selection.projectors, 262144)
					snapshot := ps6140ConstructorFlags(s.context, s.owner, s.ownerType, selection.flags, 262144)
					flags := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, s.ownerType, 262144)
					if collection == nil || flags == nil || len(ps6140PublicWorkspaceUses(pkg, collection, flags, s.ownerType, s.workspace, s.slot, contract.UnusedFormal, 262144)) != 10 || ps6140ConstructorAllocation(s, 262144) != nil {
						t.Fatal("after quietness was not isolated to genuine eager allocation removal")
					}
				}
				contract.ProfileRows, contract.ProfileWidth = 0, 0
				if (ps6140UnusedProjectionSource(pass, pkg, &contract, 262144) != nil) != (proof != nil) {
					t.Fatal("illustrative profile dimensions changed source admission")
				}
				if ps6140UnusedProjectionSource(pass, pkg, &contract, 0) != nil || ps6140UnusedProjectionSource(nil, pkg, &contract, 262144) != nil || ps6140UnusedProjectionSource(pass, nil, &contract, 262144) != nil || ps6140UnusedProjectionSource(pass, pkg, nil, 262144) != nil {
					t.Fatal("missing analysis inputs or exhausted budget admitted")
				}
			})
		}
	}
}
