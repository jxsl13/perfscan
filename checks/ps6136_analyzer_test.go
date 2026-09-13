package checks

import (
	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"strings"
	"testing"
)

func ps6136CompleteOwnerContract(pkg *ssa.Package, owner string) config.OutputWorkspaceContract {
	c := ps6136NativeOwnerContract(pkg, owner)
	local := "github.com/jxsl13/goai/llamagpu."
	constructor, entry, head, model := "newGPTDecoder", "NewGPT", "Head", "GPT"
	c.ProjectorField = "head"
	c.AllocationFunction = local + constructor
	if owner == "Decoder" {
		constructor, entry, head, model = "newDecoder", "New", "Out", "Llama"
		c.ProjectorField = "out"
		c.AllocationFunction = local + "Decoder.allocScratch"
	}
	c.FactoryFunction = local + constructor
	c.ConstructorEntries = []string{local + entry}
	c.ModelType = "github.com/jxsl13/goai/nlp." + model
	c.ModelConfigField = "Config"
	c.ModelHeadField = head
	c.ConfigMaximumRowsField = "Ctx"
	c.ConfigWidthField = "Vocab"
	c.HeadShapeMethod = "github.com/jxsl13/goai/tensor.Tensor.Shape"
	c.WorkspaceField = "logits"
	c.SlotBufferField = "b"
	c.MaximumRowsField = "maxLen"
	c.WidthField = "v"
	c.RetainedListField = "all"
	c.BackendOpsField = "ops"
	c.BackendAllocatorField = "newBuffer"
	c.BackendAllocators = []string{"github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32"}
	c.ReleaseMethod = local + owner + ".Release"
	c.RetainedBufferReleaseMethod = local + "buffer.Release"
	c.NativeBufferReleaseMethod = "github.com/jxsl13/goai/backend/metal.DeviceBuffer.Release"
	c.NativeLifetimeSemanticsReviewed = true
	c.DominantOneRowPolicyReviewed = true
	c.ExplicitRareBulkPolicyReviewed = true
	c.FreshFloat32OwnershipReviewed = true
	c.ModelProjectorWidthReviewed = true
	c.SequentialLifecycleReviewed = true
	c.CheckedShapeArithmeticReviewed = true
	c.AllProviderBuildPathsReviewed = true
	c.ExternalObservationsReviewed = true
	c.NativeAccessSemanticsReviewed = true
	for index := range c.Leaves {
		if c.Leaves[index].Kind == "capacity-transfer" {
			c.Leaves[index].HostStorageMethod = "github.com/jxsl13/goai/tensor.Tensor.Storage"
			c.Leaves[index].HostFloat32Method = "github.com/jxsl13/goai/tensor.Storage.F32"
		}
	}
	return c
}

func TestPS6136RegisteredAuthenticOwnerBeforeAfter(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, owner := range []string{"GPTDecoder", "Decoder"} {
			t.Run(revision+"/"+owner, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6136CompileOwners(t, revision, true, true)
				if err != nil {
					t.Fatal(err)
				}
				pkg := ps6136FixtureSSA(fixture)
				c := ps6136CompleteOwnerContract(pkg, owner)
				if !c.Valid() {
					t.Fatal("authentic selected owner contract invalid")
				}
				if owner == "GPTDecoder" {
					c.ProfileMaximumRows = 1024
					c.ProfileWidth = 50257
				}
				var diagnostics []analysis.Diagnostic
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
				if _, err := runPS6136WithContracts(pass, []config.OutputWorkspaceContract{c}); err != nil {
					t.Fatal(err)
				}
				want := 0
				if revision == "before" {
					want = 1
				}
				if len(diagnostics) != want {
					t.Fatalf("registered owner findings %d want %d", len(diagnostics), want)
				}
				if want > 0 {
					if !strings.Contains(diagnostics[0].Message, "rare "+c.BulkMethod) || !strings.Contains(diagnostics[0].Message, "capacity-wide physical transfers are separate") {
						t.Fatal("bulk/physical policy missing")
					}
					if owner == "GPTDecoder" && !strings.Contains(diagnostics[0].Message, "205852672 output bytes") {
						t.Fatal("actual profile retained-byte evidence missing")
					}
				}
			})
		}
	}
	if !PS6136.NeedsConfig || PS6136.AutoFix {
		t.Fatal("unsafe autonomous registration")
	}
}

func TestPS6136UnconfiguredSkipsSSA(t *testing.T) {
	t.Parallel()
	if _, err := runPS6136WithContracts(&analysis.Pass{}, nil); err != nil {
		t.Fatal(err)
	}
}
