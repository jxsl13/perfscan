package checks

import (
	"go/types"
	"testing"
)

// Replay every original source file selected by the actual CUDA build tags.
// Only external provider APIs are scaffolds; no local owner, constructor,
// recorder adapter, batched owner or graph owner is substituted or omitted.
func TestPS6140AuthenticLoadedCUDAInventory(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		for _, targetOS := range []string{"linux", "windows"} {
			t.Run(revision+"/"+targetOS, func(t *testing.T) {
				t.Parallel()
				fixture, err := ps6140CompileLoadedCUDA(t, revision, targetOS)
				if err != nil {
					t.Fatal(err)
				}
				wanted := map[string]bool{"bert.go": true, "cuda.go": true, "cuda_batched_decoder.go": true, "cuda_graph_llama.go": true, "decoder.go": true, "gpt.go": true, "medusa.go": true, "promptlookup.go": true, "sampling_fastpath.go": true, "speculative.go": true, "t5.go": true, "t5_decoder.go": true}
				if len(fixture.files) != len(wanted) {
					t.Fatalf("CUDA original files=%d want=%d", len(fixture.files), len(wanted))
				}
				for _, file := range fixture.files {
					name := fixture.fileset.Position(file.Pos()).Filename
					if !wanted[name] {
						t.Fatalf("unexpected, duplicated or substituted local file %s", name)
					}
					delete(wanted, name)
				}
				if len(wanted) != 0 {
					t.Fatalf("missing original CUDA-selected files=%v", wanted)
				}
				pkg := ps6136FixtureSSA(fixture)
				for _, name := range []string{"NewCUDA", "NewMixtralCUDA", "NewOLMo2CUDA", "NewGemma2CUDA", "newDecoder", "newMixtralDecoder", "newOLMo2Decoder", "newGemma2Decoder"} {
					function := pkg.Func(name)
					if function == nil || len(function.Blocks) == 0 || function.Pkg != pkg {
						t.Fatalf("actual source constructor %s missing", name)
					}
				}
				if pkg.Func("New") != nil {
					t.Fatal("Metal public entry incorrectly present in CUDA fixture")
				}
				var cuda *types.Package
				for _, imported := range fixture.pkg.Imports() {
					if imported.Path() == "github.com/jxsl13/goai/backend/cuda" {
						cuda = imported
					}
				}
				if cuda == nil || pkg.Prog.Package(cuda) == nil {
					t.Fatal("actual external CUDA package metadata missing")
				}
				native := pkg.Prog.Package(cuda).Func("NewDeviceBufferF32")
				if native == nil || len(native.Blocks) != 0 || native.Syntax() != nil {
					t.Fatal("external CUDA metadata must not become analyzed native source bodies")
				}
			})
		}
	}
	if _, err := ps6140CompileLoadedCUDA(t, "before", "darwin"); err == nil {
		t.Fatal("unsupported Darwin CUDA source partition accepted")
	}
}
