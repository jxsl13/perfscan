package checks

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The fixture compiles every original non-test file selected by darwin+cgo,
// without trimming owner/adapter methods. Overrides are visibly type-only
// external APIs: no local owner implementation or native execution is mocked.
func ps6140CompileLoadedMetal(t *testing.T, revision string) (*analysisOwnerFixture, error) {
	return ps6140CompileLoadedMetalRewrite(t, revision, nil)
}

func ps6140CompileLoadedMetalRewrite(t *testing.T, revision string, rewrite func(*token.FileSet, []*ast.File) []*ast.File) (*analysisOwnerFixture, error) {
	t.Helper()
	return ps6140CompileLoadedProviderRewrite(t, revision, "darwin", "metal", rewrite)
}

func ps6140CompileLoadedCUDA(t *testing.T, revision, targetOS string) (*analysisOwnerFixture, error) {
	t.Helper()
	return ps6140CompileLoadedProviderRewrite(t, revision, targetOS, "cuda", nil)
}

func ps6140CompileLoadedProviderRewrite(t *testing.T, revision, targetOS, provider string, rewrite func(*token.FileSet, []*ast.File) []*ast.File) (*analysisOwnerFixture, error) {
	t.Helper()
	if provider != "metal" && provider != "cuda" || provider == "metal" && targetOS != "darwin" || provider == "cuda" && targetOS != "linux" && targetOS != "windows" {
		return nil, fmt.Errorf("unsupported source fixture provider/OS: %s/%s", provider, targetOS)
	}
	base, err := ps6140CompileOwner(t, revision)
	if err != nil {
		return nil, err
	}
	overrides := ps6140ExternalFixtureAPIs(base.sources)
	if provider == "cuda" {
		overrides["github.com/jxsl13/goai/backend/cuda"] = ps6140CUDAFixtureAPI()
		overrides["github.com/jxsl13/goai/backend"] += "\nconst CUDA Name=\"cuda\"\n"
		// Exact exported shapes/signatures from the same pinned provider's
		// backend/attrs.go and format/gguf/{gguf,quant,quant_matmul}.go. These
		// external declarations remain metadata, never quantizer execution.
		overrides["github.com/jxsl13/goai/backend"] = strings.Replace(overrides["github.com/jxsl13/goai/backend"],
			"type RoPEAttrs struct{Base float64;Heads int}",
			"type RoPEAttrs struct{Base,PosScale,YaRNScale,YaRNOrigCtx,YaRNBetaFast,YaRNBetaSlow float64;Heads,PosOffset int;XPos bool;XPosGamma float64;XPosDownscale bool}", 1)
		overrides["github.com/jxsl13/goai/format/gguf"] = `package gguf
import "github.com/jxsl13/goai/tensor"
type QuantType uint32
const Q4_K QuantType=12
type QuantTensor struct{Data []byte;GGType uint32;Shape tensor.Shape}
func Quantize(t *tensor.Tensor,qt QuantType)([]byte,error)
func(q QuantTensor)Dequantize()(*tensor.Tensor,error)
`
	}
	var loadError error
	fixture, err := ps6136CompileOwnersRewriteImports(t, "after", true, true, func(fs *token.FileSet, _ []*ast.File) []*ast.File {
		data, err := os.ReadFile("testdata/ps6140-owner/common/manifest.json")
		if err != nil {
			loadError = err
			return nil
		}
		var manifest []struct{ Name, SHA string }
		if err := json.Unmarshal(data, &manifest); err != nil {
			loadError = err
			return nil
		}
		var files []*ast.File
		for _, item := range append(manifest, struct{ Name, SHA string }{Name: "decoder.go"}) {
			path := "testdata/ps6140-owner/common/" + item.Name + ".txt"
			if item.Name == "decoder.go" {
				path = "testdata/ps6140-owner/" + revision + "/decoder.go.txt"
			}
			source, err := os.ReadFile(path)
			if err != nil {
				loadError = err
				return nil
			}
			first, _, _ := strings.Cut(string(source), "\n")
			if strings.HasPrefix(first, "//go:build ") {
				expression, err := constraint.Parse(first)
				if err != nil {
					loadError = err
					return nil
				}
				if !expression.Eval(func(tag string) bool { return tag == targetOS || tag == "cgo" || provider == "cuda" && tag == "cuda" }) {
					continue
				}
			}
			file, err := parser.ParseFile(fs, item.Name, source, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				loadError = err
				return nil
			}
			files = append(files, file)
		}
		if rewrite != nil {
			files = rewrite(fs, files)
		}
		return files
	}, overrides)
	if loadError != nil {
		return nil, loadError
	}
	return fixture, err
}

func TestPS6140AuthenticLoadedMetalInventory(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, revision)
			if err != nil {
				t.Fatal(err)
			}
			if len(fixture.files) != 10 {
				t.Fatalf("loaded original files=%d want10", len(fixture.files))
			}
			wanted := map[string]bool{"bert.go": true, "decoder.go": true, "gpt.go": true, "llamagpu.go": true, "medusa.go": true, "promptlookup.go": true, "sampling_fastpath.go": true, "speculative.go": true, "t5.go": true, "t5_decoder.go": true}
			for _, file := range fixture.files {
				name := fixture.fileset.Position(file.Pos()).Filename
				if !wanted[name] {
					t.Fatalf("unexpected/local scaffold file %s", name)
				}
				delete(wanted, name)
			}
			if len(wanted) != 0 {
				t.Fatalf("missing originals=%v", wanted)
			}
		})
	}
}

// These declarations type-check source-only observations. Their zero bodies
// cannot establish native/model semantics and are outside the local SSA package.
func ps6140ExternalFixtureAPIs(base map[string]string) map[string]string {
	result := make(map[string]string)
	nlp := strings.Replace(base["github.com/jxsl13/goai/nlp"], `import(`, `import("math/rand/v2";`, 1)
	result["github.com/jxsl13/goai/nlp"] = nlp + `
type SpecStats struct{Proposed,Accepted int}
type MedusaHeads struct{W []*tensor.Tensor}
const MedusaEpsilon,MedusaDelta=1.0,1.0
func TypicalAcceptance([]float64,int,float64,float64)bool{return false}
func SpeculativeRun([][]float64,[][]float64,[]int,*rand.Rand)[]int{return nil}
func NgramLookup([]int,int,int)[]int{return nil}
func(*Sampler)Dist([]float64)[]float64{return nil}
type BertConfig struct{Dim,Heads,MaxPos,PosOffset,Vocab,TypeVocab int;Eps float64}
type BertLayer struct{Attn *MHA;AttnLN,FFNLN *nn.LayerNorm;W1,B1,W2,B2 *tensor.Tensor}
type Bert struct{Config BertConfig;Layers []*BertLayer;TokEmb,PosEmb,SegEmb *tensor.Tensor;EmbLN *nn.LayerNorm}
type T5Config struct{Dim,Heads,HeadDim,FFN,Vocab int;Eps float64}
type T5Block struct{AttnNorm,FFNNorm *nn.RMSNorm;Wq,Wk,Wv,Wo,Wi0,Wi1,WOut *tensor.Tensor}
type T5 struct{Config T5Config;Blocks []*T5Block;Shared *tensor.Tensor;RelBias *nn.T5RelativeBias;FinalNorm *nn.RMSNorm}
type T5DecoderBlock struct{SelfNorm,CrossNorm,FFNNorm *nn.RMSNorm;SWq,SWk,SWv,SWo,CWq,CWk,CWv,CWo,Wi0,Wi1,WOut *tensor.Tensor}
type T5Decoder struct{Config T5Config;Blocks []*T5DecoderBlock;Shared,LMHead *tensor.Tensor;RelBias *nn.T5RelativeBias;FinalNorm *nn.RMSNorm}
`
	result["github.com/jxsl13/goai/tensor"] = base["github.com/jxsl13/goai/tensor"] + `
func(*Tensor)Ndim()int{return 0}
func(*Tensor)Permute(...int)(*Tensor,error){return nil,nil}
func(t *Tensor)Contiguous()*Tensor{return t}
func(*Storage)F64()[]float64{return nil}
`
	result["github.com/jxsl13/goai/backend"] = base["github.com/jxsl13/goai/backend"] + `
type Context struct{}
func NewContext()*Context{return nil}
func(c *Context)WithBackend(Kind)*Context{return c}
func Reference()Kind{return "reference"}
`
	result["github.com/jxsl13/goai/nn"] = base["github.com/jxsl13/goai/nn"] + `
type T5RelativeBias struct{}
func(*T5RelativeBias)Bias(*backend.Context,int,int)(*tensor.Tensor,error){return nil,nil}
func(*T5RelativeBias)BiasRow(*backend.Context,int,int)(*tensor.Tensor,error){return nil,nil}
`
	result["github.com/jxsl13/goai/backend/metal"] = ps6140MetalFixtureAPI(base["github.com/jxsl13/goai/backend/metal"])
	return result
}

func ps6140MetalFixtureAPI(base string) string {
	return base + `
func(*Recorder)Barrier()error{return nil}
func(*Recorder)RMSNorm(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,float32)error{return nil}
func(*Recorder)LayerNorm(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,float32)error{return nil}
func(*Recorder)BiasGELU(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int)error{return nil}
func(*Recorder)MatMulAcc(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int)error{return nil}
func(*Recorder)MatMulStridedB(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int)error{return nil}
func(*Recorder)RoPE(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)RoPEAt(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)RoPEPair(*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)RoPEPairSplit(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)Blit(*DeviceBuffer,int,*DeviceBuffer,int,int)error{return nil}
func(*Recorder)Copy2D(*DeviceBuffer,int,int,*DeviceBuffer,int,int,int,int)error{return nil}
func(*Recorder)Copy2DF32ToF16Pair(*DeviceBuffer,int,int,*DeviceBuffer,int,int,*DeviceBuffer,int,int,*DeviceBuffer,int,int,int,int)error{return nil}
func(*Recorder)RoPEF16KVAppend(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)RoPEPairF16KVAppend(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)MHA(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)MHAF16KV(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int,int,int,int,int,int,float32)error{return nil}
func(*Recorder)Unary(*DeviceBuffer,*DeviceBuffer,int)error{return nil}
func(*Recorder)Binary(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int)error{return nil}
func(*Recorder)BinaryN(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int)error{return nil}
func(*Recorder)Commit()error{return nil}
func(*Recorder)Free(){}
type ResidentQWeight struct{};func(*ResidentQWeight)Close()error{return nil}
type ResidentQGroup struct{};func(*ResidentQGroup)Close()error{return nil}
func(*Recorder)QMatMulResident(*DeviceBuffer,*ResidentQWeight,*DeviceBuffer,int)error{return nil}
func(*Recorder)QMatMulResidentGroup(*DeviceBuffer,*ResidentQGroup,*DeviceBuffer,int)error{return nil}
func NewResidentQGroup(...*ResidentQWeight)(*ResidentQGroup,error){return nil,nil}
type Backend struct{};func(Backend)UploadQuant([]byte,uint32,int,int)(interface{},error){return nil,nil}
` + ps6140QuantUploadFixtureAPIs()
}

func ps6140QuantUploadFixtureAPIs() string {
	var source strings.Builder
	for _, format := range []string{"Q4_1", "IQ2_XXS", "IQ2_XS", "IQ3_XXS", "IQ1_S", "IQ4_NL", "IQ3_S", "IQ2_S", "IQ4_XS", "IQ1_M", "TQ1_0", "TQ2_0", "MXFP4", "Q1_0"} {
		source.WriteString("func UploadQWeight" + format + "([]byte,int,int)(*ResidentQWeight,error){return nil,nil}\n")
	}
	return source.String()
}
