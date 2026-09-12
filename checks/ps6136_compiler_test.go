package checks

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// The original complete GPT file is compiled without replacing any owner
// body. Supporting declarations and external APIs are explicit scaffolding;
// this is source analysis, not execution or proof of opaque backend semantics.
func ps6136CompileGPT(t *testing.T, revision string) (*analysisOwnerFixture, error) {
	return ps6136CompileGPTBackend(t, revision, false)
}

func ps6136CompileGPTBackend(t *testing.T, revision string, metalEntry bool) (*analysisOwnerFixture, error) {
	return ps6136CompileOwners(t, revision, metalEntry, false)
}

func ps6136CompileOwners(t *testing.T, revision string, metalEntry, shared bool) (*analysisOwnerFixture, error) {
	return ps6136CompileOwnersRewrite(t, revision, metalEntry, shared, nil)
}

// Rewrite is only for explicitly altered adversaries. Authentic replay calls
// the nil-rewrite entry above and still compiles complete original owner bodies.
func ps6136CompileOwnersRewrite(t *testing.T, revision string, metalEntry, shared bool, rewrite func(*token.FileSet, []*ast.File) []*ast.File) (*analysisOwnerFixture, error) {
	t.Helper()
	fs := token.NewFileSet()
	data, err := os.ReadFile("testdata/ps6136-owner/" + revision + "/gpt.go.txt")
	if err != nil {
		return nil, err
	}
	gpt, err := parser.ParseFile(fs, "gpt.go", data, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	decoderSource, err := os.ReadFile("testdata/ps6136-owner/" + revision + "/decoder.go.txt")
	if err != nil {
		return nil, err
	}
	decoder, err := parser.ParseFile(fs, "decoder.go", decoderSource, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	support, err := parser.ParseFile(fs, "support.go", `package llamagpu
import "github.com/jxsl13/goai/tensor"
func flat1D(*tensor.Tensor)[]float32{return nil}
func flat2D(*tensor.Tensor)[]float32{return nil}
func embedRow(*tensor.Tensor,int,int)[]float32{return nil}
func(l f32Linear)recordMoE(recorder,buffer,buffer,int,buffer,int,int)error{return nil}
`, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	wantedTypes := map[string]bool{"buffer": true, "qweight": true, "linear": true, "recorder": true, "f32Linear": true, "backendOps": true, "bufSlot": true, "growBuffer": true}
	wantedFunctions := map[string]bool{"firstErr": true, "logitsForRows": true}
	if shared {
		wantedFunctions["newDecoder"] = true
		wantedFunctions["newDecoderCommon"] = true
		wantedFunctions["recordBarrier"] = true
		wantedFunctions["boolToInt"] = true
		extra, err := parser.ParseFile(fs, "shared-scaffold.go", `package llamagpu
import("fmt";"math";"github.com/jxsl13/goai/backend";"github.com/jxsl13/goai/nlp";"github.com/jxsl13/goai/nn")
var _=fmt.Errorf;var _=math.Sqrt;var _=backend.RoPEFreqs;var _ *nn.RMSNorm
func(q quantLinear)recordAdd(recorder,buffer,buffer,buffer,int,int)error{return nil}
func(q quantLinear)recordMoE(recorder,buffer,buffer,int,buffer,int,int)error{return nil}
func fastTopKSampler(nlp.TokenSampler,int)(*nlp.Sampler,int){return nil,0}
func fastTopPSampler(nlp.TokenSampler,int)(*nlp.Sampler,int){return nil,0}
func sampleTopKCandidates(*nlp.Sampler,[]int32,[]float32)int{return 0}
`, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		// Owner bodies keep their original import binding environment.
		for _, declaration := range extra.Decls {
			support.Decls = append(support.Decls, declaration)
		}
		support.Imports = append(support.Imports, extra.Imports...)
		deviceSource, err := os.ReadFile("testdata/ps6136-owner/sampling_fastpath.go.txt")
		if err != nil {
			return nil, err
		}
		device, err := parser.ParseFile(fs, "sampling_fastpath.go", deviceSource, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, declaration := range device.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typ := spec.(*ast.TypeSpec)
				if typ.Name.Name == "deviceTopKer" || typ.Name.Name == "deviceTopPer" {
					support.Decls = append(support.Decls, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{typ}})
				}
			}
		}
	}
	for _, declaration := range decoder.Decls {
		switch n := declaration.(type) {
		case *ast.GenDecl:
			if n.Tok == token.CONST {
				support.Decls = append(support.Decls, n)
				continue
			}
			for _, spec := range n.Specs {
				if typ, ok := spec.(*ast.TypeSpec); ok && (shared || wantedTypes[typ.Name.Name]) {
					support.Decls = append(support.Decls, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{typ}})
				}
			}
		case *ast.FuncDecl:
			wanted := wantedFunctions[n.Name.Name]
			if n.Recv != nil && len(n.Recv.List) == 1 {
				receiver := n.Recv.List[0].Type
				if ptr, ok := receiver.(*ast.StarExpr); ok {
					receiver = ptr.X
				}
				if typ, ok := receiver.(*ast.Ident); ok {
					wanted = wanted || typ.Name == "growBuffer" || typ.Name == "f32Linear" && (n.Name.Name == "record" || n.Name.Name == "recordAdd")
					wanted = wanted || shared && (typ.Name == "Decoder" || typ.Name == "quantLinear" && n.Name.Name == "record")
				}
			}
			if wanted {
				support.Decls = append(support.Decls, n)
			}
		}
	}
	imports := &ps6136FixtureImporter{fallback: importer.Default(), packages: map[string]*types.Package{}, sources: map[string]string{}}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	files := []*ast.File{gpt, support}
	if shared {
		// Complete, unmodified owner files: no alternate constructor or
		// allocation/flatten helper is omitted from the coverage replay.
		samplingSource, err := os.ReadFile("testdata/ps6136-owner/sampling_fastpath.go.txt")
		if err != nil {
			return nil, err
		}
		sampling, err := parser.ParseFile(fs, "sampling_fastpath.go", samplingSource, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		files = []*ast.File{gpt, decoder, sampling}
	}
	if metalEntry {
		data, err := os.ReadFile("testdata/ps6136-owner/llamagpu.go.txt")
		if err != nil {
			return nil, err
		}
		adapter, err := parser.ParseFile(fs, "llamagpu.go", data, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		var declarations []ast.Decl
		for _, declaration := range adapter.Decls {
			switch n := declaration.(type) {
			case *ast.GenDecl:
				if n.Tok == token.IMPORT {
					declarations = append(declarations, n)
					continue
				}
				for _, spec := range n.Specs {
					if typ, ok := spec.(*ast.TypeSpec); ok && (typ.Name.Name == "mBuf" || typ.Name.Name == "mRec" || shared && typ.Name.Name == "mProfileRec") {
						declarations = append(declarations, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{typ}})
					}
				}
			case *ast.FuncDecl:
				if n.Name.Name == "NewGPT" || shared && n.Name.Name == "New" {
					declarations = append(declarations, n)
				}
				if shared && (n.Name.Name == "NewQuant" || n.Name.Name == "NewQuantF16KV" || n.Name.Name == "newQuantMetal" || n.Name.Name == "newQuantMetalWithMixedQKV" || n.Name.Name == "concurrentMetalDecodeEligible" || n.Name.Name == "ProfileMetalStep") {
					declarations = append(declarations, n)
				}
				if shared && n.Recv != nil && (n.Name.Name == "Finish" || n.Name.Name == "Wait") {
					if receiver, ok := n.Recv.List[0].Type.(*ast.Ident); ok && receiver.Name == "mProfileRec" {
						declarations = append(declarations, n)
					}
				}
				if n.Name.Name == "mb" || n.Recv != nil && (n.Name.Name == "MatMul" || n.Name.Name == "AddBias") {
					declarations = append(declarations, n)
				}
			}
		}
		adapter.Decls = declarations
		files = append(files, adapter)
		if shared {
			// Type-only support for an unselected quantized-provider callback;
			// this is not source/execution proof of its native implementation.
			quantized, err := parser.ParseFile(fs, "quantized-provider-scaffold.go", `package llamagpu;func metalUploadQWeight([]byte,uint32,int,int)(qweight,error){return nil,nil};func metalGroupQWeights([]qweight)(qweight,error){return nil,nil}`, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			files = append(files, quantized)
		}
		for _, declaration := range decoder.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok || typ.Name.Name != "recorder" {
					continue
				}
				api := typ.Type.(*ast.InterfaceType)
				for _, method := range api.Methods.List {
					if method.Names[0].Name == "MatMul" || method.Names[0].Name == "AddBias" {
						continue // The complete original forwarding bodies are above.
					}
					var signature bytes.Buffer
					if err := format.Node(&signature, fs, method.Type); err != nil {
						return nil, err
					}
					body := ps6136ZeroReturnSource(fs, method.Type.(*ast.FuncType))
					generated, err := parser.ParseFile(fs, "recorder-scaffold.go", "package llamagpu;func(mRec) "+method.Names[0].Name+strings.TrimPrefix(signature.String(), "func")+"{"+body+"}", parser.SkipObjectResolution)
					if err != nil {
						return nil, err
					}
					files = append(files, generated)
				}
			}
		}
	}
	if rewrite != nil {
		files = rewrite(fs, files)
	}
	pkg, err := (&types.Config{Importer: imports}).Check("github.com/jxsl13/goai/llamagpu", fs, files, info)
	return &analysisOwnerFixture{fs, files, info, pkg, imports.sources}, err
}

type analysisOwnerFixture struct {
	fileset *token.FileSet
	files   []*ast.File
	info    *types.Info
	pkg     *types.Package
	sources map[string]string
}

type ps6136FixtureImporter struct {
	fallback types.Importer
	packages map[string]*types.Package
	sources  map[string]string
}

func (i *ps6136FixtureImporter) Import(path string) (*types.Package, error) {
	if pkg := i.packages[path]; pkg != nil {
		return pkg, nil
	}
	var source string
	switch path {
	case "github.com/jxsl13/goai/tensor":
		source = `package tensor
type Tensor struct{};type Shape []int;type Dtype uint8;const F32 Dtype=1
type Storage struct{};func(*Storage)F32()[]float32{return nil}
func(*Tensor)Shape()Shape{return nil};func(*Tensor)Storage()*Storage{return nil}
func(*Tensor)AtF64(...int)float64{return 0}
func(*Tensor)Slice(int,int,int)(*Tensor,error){return nil,nil}
func(*Tensor)Dtype()Dtype{return F32};func(*Tensor)IsContiguous()bool{return true}
func(*Tensor)Offset()int{return 0};func(t *Tensor)Cast(Dtype)*Tensor{return t}
func New(Dtype,Shape)*Tensor{return nil}
func(*Tensor)Transpose(int,int)(*Tensor,error){return nil,nil}
`
	case "github.com/jxsl13/goai/nlp":
		api, err := os.ReadFile("testdata/ps6136-owner/model-api.go.txt")
		if err != nil {
			return nil, err
		}
		source = string(api) + `
type TokenSampler interface{SampleWithHistory([]float64,[]int)int}
type Sampler struct{Temperature,TopP,MinP,Epsilon,Eta,Typical,RepeatPenalty,FreqPenalty,PresencePenalty,DRYMultiplier,XTCProbability float64;TopK,TopNSigma int}
func(*Sampler)Sample([]float64)int{return 0}
func(*Sampler)SampleWithHistory([]float64,[]int)int{return 0}
func(*Sampler)SampleTopPFromCandidates([]float64,[]int32,float64,float64)(int,bool){return 0,false}
`
	case "github.com/jxsl13/goai/backend":
		source = `package backend;type Kind string;type Name string;type Op int;type ResidentWeight interface{};const Metal Kind="metal";var ErrQuantUnsupported error
type RoPEAttrs struct{Base float64;Heads int}
func RoPEFreqs(int,RoPEAttrs)([]float64,float64){return nil,0}
func ALiBiSlopes(int)[]float64{return nil}
`
	case "github.com/jxsl13/goai/nn":
		api, err := os.ReadFile("testdata/ps6136-owner/nn-api.go.txt")
		if err != nil {
			return nil, err
		}
		source = string(api)
	case "github.com/jxsl13/goai/format/gguf":
		source = `package gguf;type QuantType uint32`
	case "github.com/jxsl13/goai/backend/metal":
		source = `package metal
type DeviceBuffer struct{}
func(*DeviceBuffer)UploadF32([]float32)error{return nil}
func(*DeviceBuffer)DownloadF32([]float32)error{return nil}
func(*DeviceBuffer)Release(){}
func(*DeviceBuffer)Len()int{return 0};func(*DeviceBuffer)ByteLen()int{return 0}
func(*DeviceBuffer)TopKN(int,int)([]int32,[]float32,error){return nil,nil,nil}
type Recorder struct{}
type RecorderProfile struct{}
func(*Recorder)MatMul(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int,int)error{return nil}
func(*Recorder)AddBias(*DeviceBuffer,*DeviceBuffer,*DeviceBuffer,int,int)error{return nil}
func(*Recorder)Finish()error{return nil};func(*Recorder)Wait()error{return nil}
func(*Recorder)Profile()(RecorderProfile,error){return RecorderProfile{},nil}
func Available()bool{return true}
func NewDeviceBufferF32([]float32)(*DeviceBuffer,error){return nil,nil}
func NewDeviceBufferF16Zeros(int)(*DeviceBuffer,error){return nil,nil}
func NewRecorder()(*Recorder,error){return nil,nil}
func NewConcurrentRecorder()(*Recorder,error){return nil,nil}
func NewProfilingRecorder(int)(*Recorder,error){return nil,nil}
`
	default:
		return i.fallback.Import(path)
	}
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, path+".go", source, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	pkg, err := (&types.Config{Importer: i}).Check(path, fs, []*ast.File{file}, nil)
	if err == nil {
		i.packages[path] = pkg
		if i.sources != nil {
			i.sources[path] = source
		}
	}
	return pkg, err
}

func ps6136ZeroReturnSource(fs *token.FileSet, signature *ast.FuncType) string {
	if signature.Results == nil {
		return ""
	}
	var source strings.Builder
	var names []string
	for _, field := range signature.Results.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		var typ bytes.Buffer
		if err := format.Node(&typ, fs, field.Type); err != nil {
			panic(err)
		}
		for range count {
			name := "result" + string(rune('A'+len(names)))
			source.WriteString("var " + name + " " + typ.String() + ";")
			names = append(names, name)
		}
	}
	source.WriteString("return " + strings.Join(names, ","))
	return source.String()
}

func TestPS6136CompleteGPTCompilerFixture(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			if _, err := ps6136CompileGPT(t, revision); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPS6136CompleteSharedDecoderCompilerFixture(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, false, true)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			if pkg.Func("newDecoder") == nil {
				t.Fatal("actual shared constructor absent")
			}
			for _, name := range []string{"newDecoderCommon", "newStableLMDecoder", "newCohereDecoder", "newNemotronDecoder", "newGemmaDecoder", "newDeepSeekV2Decoder", "newMambaDecoder", "newJambaDecoder", "newMamba2Decoder", "newRWKVDecoder", "newFalconDecoder", "newOLMo2Decoder", "newGemma2Decoder", "newMPTDecoder", "newMixtralDecoder", "newQwen2MoEDecoder", "newGraniteMoEDecoder", "newOLMoEDecoder", "newStarCoder2Decoder", "newPhiDecoder", "newGPTNeoXDecoder", "newQuantDecoder"} {
				if pkg.Func(name) == nil {
					t.Fatalf("actual alternate constructor %s absent", name)
				}
			}
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			for _, name := range []string{"allocScratch", "allocMambaScratch", "allocMamba2Scratch", "allocRWKVScratch", "mkBuf", "recordLogits", "encodeStep", "stepInto", "stepN", "Step", "StepN", "StepNLast", "Generate", "Release"} {
				if pkg.Prog.LookupMethod(types.NewPointer(owner), pkg.Pkg, name) == nil {
					t.Fatalf("actual owner method %s absent", name)
				}
			}
		})
	}
}

func ps6136FixtureSSA(fixture *analysisOwnerFixture) *ssa.Package {
	program := ssa.NewProgram(fixture.fileset, ssa.SanityCheckFunctions)
	seen := map[*types.Package]bool{}
	var create func(*types.Package)
	create = func(pkg *types.Package) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		for _, imported := range pkg.Imports() {
			create(imported)
		}
		if pkg != fixture.pkg {
			program.CreatePackage(pkg, nil, nil, true)
		}
	}
	create(fixture.pkg)
	converted := program.CreatePackage(fixture.pkg, fixture.files, fixture.info, true)
	converted.Build()
	return converted
}

func TestPS6136AuthenticGPTConstructorFactory(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileGPT(t, revision)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			constructor := pkg.Func("newGPTDecoder")
			root := ps6125NewSSAContext(constructor, nil, nil, 32)
			owner := pkg.Pkg.Scope().Lookup("GPTDecoder").Type().(*types.Named)
			fieldObject, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "logits")
			workspace := fieldObject.(*types.Var)
			listObject, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "all")
			list := listObject.(*types.Var)
			allocatorObject, _, _ := types.LookupFieldOrMethod(pkg.Pkg.Scope().Lookup("backendOps").Type(), true, pkg.Pkg, "newBuffer")
			allocator := allocatorObject.(*types.Var)
			slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
			paths := ps6125AccessPaths{flow: root.flow}
			found := 0
			for _, block := range constructor.Blocks {
				for _, instruction := range block.Instrs {
					store, ok := instruction.(*ssa.Store)
					if !ok || !root.flow.blocks[block] {
						continue
					}
					path := paths.resolve(store.Addr)
					if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != workspace {
						continue
					}
					call, ok := store.Val.(*ssa.Call)
					if !ok || len(call.Call.Args) != 1 {
						t.Fatal("actual workspace store lost factory input")
					}
					factory := root.call(call)
					if factory == nil {
						t.Fatal("actual captured factory context unavailable")
					}
					backend := ps6136FactoryResult(factory, root.reference(call.Call.Args[0]), allocator, slot)
					if backend == nil {
						t.Fatal("actual backend input and returned slot provenance unproved")
					}
					if cell := ps6136FactoryErrorCell(factory, backend); cell.value == nil || cell.context != root {
						t.Fatal("actual allocator failure does not feed the same constructor error cell")
					}
					identity := ps6136AccessOwnerRoot(root, path, owner)
					method, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "Release")
					if !ps6136ConstructorErrorBarrier(root, identity, owner, ps6136FactoryErrorCell(factory, backend), ps6090FunctionID(method.(*types.Func))) {
						t.Fatal("actual same-owner final failure release/publication barrier unproved")
					}
					if !ps6136FactoryRetention(factory, backend, identity, owner, list, slot) {
						t.Fatal("actual source result is not exclusively appended to same-owner retention and returned slot")
					}
					found++
				}
			}
			if found != 1 {
				t.Fatalf("actual constructor workspace factory count %d", found)
			}
		})
	}
}

func TestPS6136AuthenticPublicGPTBackendBinding(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileGPTBackend(t, revision, true)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			entry := ps6125NewSSAContext(pkg.Func("NewGPT"), nil, nil, 64)
			constructor := pkg.Func("newGPTDecoder")
			var root *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == constructor {
					root = entry.call(call)
				}
			}
			if root == nil {
				t.Fatal("actual public constructor entry binding unavailable")
			}
			owner := pkg.Pkg.Scope().Lookup("GPTDecoder").Type().(*types.Named)
			workspaceObject, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "logits")
			allocatorObject, _, _ := types.LookupFieldOrMethod(pkg.Pkg.Scope().Lookup("backendOps").Type(), true, pkg.Pkg, "newBuffer")
			slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
			paths := ps6125AccessPaths{flow: root.flow}
			found := 0
			for _, block := range constructor.Blocks {
				for _, instruction := range block.Instrs {
					store, ok := instruction.(*ssa.Store)
					if !ok || !root.flow.blocks[block] {
						continue
					}
					path := paths.resolve(store.Addr)
					if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != workspaceObject {
						continue
					}
					call := store.Val.(*ssa.Call)
					input := root.reference(call.Call.Args[0])
					factory := root.call(call)
					backend := ps6136FactoryResult(factory, input, allocatorObject.(*types.Var), slot)
					if backend == nil || !ps6136BoundAllocator(factory, backend, input, map[string]bool{"github.com/jxsl13/goai/backend/metal.NewDeviceBufferF32": true}) {
						t.Fatal("actual public entry -> allocation callback -> native allocator descriptor/result binding unproved")
					}
					if ps6136BoundAllocator(factory, backend, input, map[string]bool{"other/backend.NewDeviceBufferF32": true}) {
						t.Fatal("wrong exact backend identity accepted")
					}
					found++
				}
			}
			if found != 1 {
				t.Fatalf("public constructor allocation count %d", found)
			}
		})
	}
}

func TestPS6136AuthenticSharedConstructorFactory(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6136CompileOwners(t, revision, false, true)
			if err != nil {
				t.Fatal(err)
			}
			pkg := ps6136FixtureSSA(fixture)
			owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
			workspace, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "logits")
			list, _, _ := types.LookupFieldOrMethod(owner, true, pkg.Pkg, "all")
			allocator, _, _ := types.LookupFieldOrMethod(pkg.Pkg.Scope().Lookup("backendOps").Type(), true, pkg.Pkg, "newBuffer")
			slot := pkg.Pkg.Scope().Lookup("bufSlot").Type().Underlying().(*types.Struct).Field(0)
			root := ps6125NewSSAContext(pkg.Func("newDecoder"), nil, nil, 96)
			found := 0
			var visit func(*ps6125SSAContext)
			visit = func(context *ps6125SSAContext) {
				paths := ps6125AccessPaths{flow: context.flow}
				for _, block := range context.flow.function.Blocks {
					if !context.flow.blocks[block] {
						continue
					}
					for _, instruction := range block.Instrs {
						store, ok := instruction.(*ssa.Store)
						if !ok {
							continue
						}
						path := paths.resolve(store.Addr)
						if !path.known || len(path.access.fields) != 1 || path.access.fields[0] != workspace {
							continue
						}
						call, ok := store.Val.(*ssa.Call)
						if !ok || len(call.Call.Args) != 1 {
							t.Fatal("shared allocation lost actual factory input")
						}
						factory := ps6136Call(context, call)
						if factory == nil {
							t.Fatalf("shared mkBuf returned closure context unavailable: value %T %s; reference %T %v", call.Call.Value, call.Call.Value, context.reference(call.Call.Value).value, context.reference(call.Call.Value).value)
						}
						backend := ps6136FactoryResult(factory, context.reference(call.Call.Args[0]), allocator.(*types.Var), slot)
						identity := ps6136AccessOwnerRoot(context, path, owner)
						if backend == nil || !ps6136FactoryRetention(factory, backend, identity, owner, list.(*types.Var), slot) {
							factoryPaths := ps6125AccessPaths{flow: factory.flow}
							for _, factoryBlock := range factory.flow.function.Blocks {
								for _, factoryInstruction := range factoryBlock.Instrs {
									if write, ok := factoryInstruction.(*ssa.Store); ok {
										p := factoryPaths.resolve(write.Addr)
										if p.known && len(p.access.fields) == 1 && p.access.fields[0] == list {
											t.Logf("retention root %v path %v", ps6136AccessOwnerRoot(factory, p, owner), p)
										}
									}
								}
							}
							t.Fatalf("actual shared factory input/result and same-owner retention unproved: backend %v identity %v", backend, identity)
						}
						found++
					}
				}
				for _, call := range ps6125ContextCalls(context) {
					callee := call.Call.StaticCallee()
					if callee != nil && callee.Name() == "allocScratch" {
						child := context.call(call)
						if child == nil {
							t.Fatal("shared constructor allocScratch context unavailable")
						}
						visit(child)
					}
				}
			}
			visit(root)
			if found != 1 {
				t.Fatalf("shared constructor workspace allocations %d", found)
			}
		})
	}
}
