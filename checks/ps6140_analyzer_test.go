package checks

import (
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

func ps6140CUDAContractsForTest(revision string) []config.UnusedProjectionScratchContract {
	var contracts []config.UnusedProjectionScratchContract
	for _, class := range []struct{ model, entry, factory, concrete string }{
		{"Llama", "NewCUDA", "newDecoder", "f32Linear"},
		{"Mixtral", "NewMixtralCUDA", "newMixtralDecoder", "f32Linear"},
		{"OLMo2", "NewOLMo2CUDA", "newOLMo2Decoder", "f32Linear"},
		{"Gemma2", "NewGemma2CUDA", "newGemma2Decoder", "f32Linear"},
		{"QuantLlama", "NewQuantCUDA", "newQuantDecoder", "quantLinear"},
	} {
		for _, member := range []string{"ao", "mo"} {
			c := ps6140AuthenticSourceContract(revision, member)
			c.ModelType = "github.com/jxsl13/goai/nlp." + class.model
			c.ConstructorEntry = "github.com/jxsl13/goai/llamagpu." + class.entry
			c.ConstructorFunction = "github.com/jxsl13/goai/llamagpu." + class.factory
			c.BackendAllocator = "github.com/jxsl13/goai/backend/cuda.NewDeviceBufferF32"
			c.NativeReleaseMethod = "github.com/jxsl13/goai/backend/cuda.DeviceF32.Release"
			c.NativeAllocationCountUnit = "float32-elements"
			for index := range c.Projections {
				c.Projections[index].ConcreteType = "github.com/jxsl13/goai/llamagpu." + class.concrete
			}
			if class.model == "Mixtral" {
				c.Projections = c.Projections[:1]
				// Deliberately illustrative ranking input, not a native measurement.
				c.ProfileRows, c.ProfileWidth = 8192, 4096
			}
			contracts = append(contracts, c)
		}
	}
	return contracts
}

func TestPS6140RegisteredAuthenticSource(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, target := range []struct{ provider, os string }{{"metal", "darwin"}, {"cuda", "linux"}, {"cuda", "windows"}} {
			t.Run(revision+"/"+target.provider+"/"+target.os, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6140CompileLoadedProviderRewrite(t, revision, target.os, target.provider, nil)
				if err != nil {
					t.Fatal(err)
				}
				contracts := []config.UnusedProjectionScratchContract{ps6140AuthenticSourceContract(revision, "ao"), ps6140AuthenticSourceContract(revision, "mo")}
				if target.provider == "cuda" {
					contracts = ps6140CUDAContractsForTest(revision)
				}
				var diagnostics []analysis.Diagnostic
				pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
				if _, err := runPS6140WithContracts(pass, contracts); err != nil {
					t.Fatal(err)
				}
				want := 0
				if revision == "before" {
					want = 2
				}
				if len(diagnostics) != want {
					t.Fatalf("registered original source findings=%d want=%d", len(diagnostics), want)
				}
				for _, d := range diagnostics {
					if len(d.SuggestedFixes) != 0 || fixture.fileset.Position(d.Pos).Filename != "decoder.go" {
						t.Fatal("advisory edited code or lost original allocation position")
					}
					for _, text := range []string{"candidate unused constructor scratch", "4*Config.Ctx*Config.Dim requested bytes", "illustrative profile", "16777216 bytes", "reviewed assumptions", "not source-proved count or lifetime safety", "no automatic fix"} {
						if !strings.Contains(d.Message, text) {
							t.Fatalf("missing scoped diagnostic text %q: %s", text, d.Message)
						}
					}
					if target.provider == "cuda" {
						for _, forbidden := range []string{"NewQuantCUDA", "NewOLMo2CUDA", "NewGemma2CUDA", "backend/metal"} {
							if strings.Contains(d.Message, forbidden) {
								t.Fatalf("required-buffer or foreign-provider class reported: %s", d.Message)
							}
						}
						if strings.Contains(d.Message, "Decoder.mo:") && strings.Contains(d.Message, "NewMixtralCUDA") {
							t.Fatal("MoE accumulation scratch reported as unused")
						}
					}
				}
				if target.provider == "cuda" && revision == "before" {
					if !strings.Contains(diagnostics[0].Message, "Decoder.ao:") || !strings.Contains(diagnostics[0].Message, "NewMixtralCUDA") || !strings.Contains(diagnostics[0].Message, "134217728 bytes") {
						t.Fatal("largest illustrative request not ranked with exact active class combinations")
					}
				}
			})
		}
	}
	if PS6140.AutoFix || !PS6140.NeedsConfig || PS6140.Level != lint.LevelAggressive || PS6140.Category != "verify" {
		t.Fatal("unsafe or misclassified registered advisory")
	}
}

func TestPS6140RegisteredDuplicateAndProfileControls(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		edit func([]config.UnusedProjectionScratchContract) []config.UnusedProjectionScratchContract
		want int
	}{
		{"unconfigured", func(_ []config.UnusedProjectionScratchContract) []config.UnusedProjectionScratchContract { return nil }, 0},
		{"duplicate-class", func(c []config.UnusedProjectionScratchContract) []config.UnusedProjectionScratchContract {
			return append(c, c[0])
		}, 1},
		{"contradictory-native-class", func(c []config.UnusedProjectionScratchContract) []config.UnusedProjectionScratchContract {
			other := c[0]
			other.NativeAllocationCountUnit = "float32-elements"
			return append(c, other)
		}, 1},
		{"no-profile", func(c []config.UnusedProjectionScratchContract) []config.UnusedProjectionScratchContract {
			for i := range c {
				c[i].ProfileRows, c[i].ProfileWidth = 0, 0
			}
			return c
		}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, "before")
			if err != nil {
				t.Fatal(err)
			}
			c := test.edit([]config.UnusedProjectionScratchContract{ps6140AuthenticSourceContract("before", "ao"), ps6140AuthenticSourceContract("before", "mo")})
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
			if _, err := runPS6140WithContracts(pass, c); err != nil || len(diagnostics) != test.want {
				t.Fatalf("diagnostics=%d want=%d error=%v", len(diagnostics), test.want, err)
			}
			for _, d := range diagnostics {
				if test.want == 1 && !strings.Contains(d.Message, "Decoder.mo:") || test.name == "no-profile" && strings.Contains(d.Message, "illustrative profile") {
					t.Fatalf("contract/profile isolation failed: %s", d.Message)
				}
			}
		})
	}
}

func TestPS6140UnconfiguredSkipsSSA(t *testing.T) {
	t.Parallel()
	for _, contracts := range [][]config.UnusedProjectionScratchContract{nil, {{}}, {ps6140AuthenticSourceContract("before", "ao"), ps6140AuthenticSourceContract("before", "ao")}} {
		if _, err := runPS6140WithContracts(&analysis.Pass{}, contracts); err != nil {
			t.Fatal(err)
		}
	}
}
