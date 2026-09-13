package checks

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/ssa"
)

// Compile every authentic decoder body, including quantized fallbacks and all
// architecture constructors. External APIs remain PS6136 type-only scaffolds;
// this is not native execution or proof of unused selected-instance storage.
func ps6140CompileOwner(t *testing.T, revision string) (*analysisOwnerFixture, error) {
	t.Helper()
	var sourceError error
	fixture, err := ps6136CompileOwnersRewrite(t, "after", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
		data, readError := os.ReadFile("testdata/ps6140-owner/" + revision + "/decoder.go.txt")
		if readError != nil {
			sourceError = readError
			return files
		}
		decoder, parseError := parser.ParseFile(fs, "ps6140-decoder.go", data, parser.SkipObjectResolution)
		if parseError != nil {
			sourceError = parseError
			return files
		}
		gptData, readError := os.ReadFile("testdata/ps6140-owner/common/gpt.go.txt")
		if readError != nil {
			sourceError = readError
			return files
		}
		gpt, parseError := parser.ParseFile(fs, "ps6140-gpt.go", gptData, parser.SkipObjectResolution)
		if parseError != nil {
			sourceError = parseError
			return files
		}
		replaced := 0
		gptReplaced := 0
		for index, file := range files {
			if fs.Position(file.Pos()).Filename == "decoder.go" {
				files[index] = decoder
				replaced++
			}
			if fs.Position(file.Pos()).Filename == "gpt.go" {
				files[index] = gpt
				gptReplaced++
			}
		}
		if replaced != 1 || gptReplaced != 1 {
			sourceError = fmt.Errorf("authentic decoder/GPT replacements=%d/%d, want1/1", replaced, gptReplaced)
		}
		return files
	})
	if sourceError != nil {
		return nil, sourceError
	}
	return fixture, err
}

func TestPS6140OwnerPinIdentity(t *testing.T) {
	t.Parallel()
	for revision, identity := range map[string]string{"before": "a5227dfdd7aff3554a3fa85ea25b05c901a0fd30", "after": "5cbf2a23c31a71fbe663b39310c05bb776d4d7ee"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/ps6140-owner/" + revision + "/decoder.go.txt")
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(data, '\r') {
				t.Fatal("owner pin is not original LF source")
			}
			// SHA1 is Git's blob identity, not an authentication primitive.
			hash := sha1.New()
			fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			hash.Write(data)
			if got := fmt.Sprintf("%x", hash.Sum(nil)); got != identity {
				t.Fatalf("pin blob=%s want%s", got, identity)
			}
		})
	}
}

func TestPS6140CommonOwnerPinIdentity(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6140-owner/common/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []struct {
		Name string
		SHA  string
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 13 {
		t.Fatalf("common build inventory=%d, want13", len(manifest))
	}
	for _, file := range manifest {
		t.Run(file.Name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/ps6140-owner/common/" + file.Name + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			if bytes.ContainsRune(data, '\r') {
				t.Fatal("common source not LF")
			}
			hash := sha1.New()
			fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			hash.Write(data)
			if got := fmt.Sprintf("%x", hash.Sum(nil)); got != file.SHA {
				t.Fatalf("common blob=%s want%s", got, file.SHA)
			}
		})
	}
}

func TestPS6140AuthenticConstructorProjectionValue(t *testing.T) {
	t.Parallel()
	fixture, err := ps6140CompileOwner(t, "before")
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	entry := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 1024)
	constructor := pkg.Func("newDecoder")
	var context *ps6125SSAContext
	for _, call := range ps6125ContextCalls(entry) {
		if call.Call.StaticCallee() == constructor {
			context = entry.call(call)
		}
	}
	if context == nil {
		t.Fatal("selected public constructor invocation missing")
	}
	linear := pkg.Pkg.Scope().Lookup("f32Linear").Type().(*types.Named)
	blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
	fields := map[*types.Var]bool{ps6136FieldVar(blockType, "wo"): true, ps6136FieldVar(blockType, "wD"): true}
	paths := ps6125AccessPaths{flow: context.flow}
	unknownBackend := ps6125NewSSAContext(constructor, nil, nil, 1024)
	quantized := pkg.Pkg.Scope().Lookup("quantLinear").Type().(*types.Named)
	count := 0
	appends := 0
	for _, block := range constructor.Blocks {
		for _, instruction := range block.Instrs {
			if call, ok := instruction.(*ssa.Call); ok {
				if appended := ps6140AppendedBlock(context, call, blockType); appended.value != nil {
					for field := range fields {
						if !ps6140BlockProjectionValue(appended, field, linear, 256) {
							t.Fatalf("appended block lost actual%s projection", field.Name())
						}
					}
					appends++
				}
			}
			store, ok := instruction.(*ssa.Store)
			if !ok {
				continue
			}
			path := paths.resolve(store.Addr)
			if !path.known || len(path.access.fields) != 1 || !fields[path.access.fields[0]] {
				continue
			}
			if !ps6140ConcreteValue(context, store.Val, linear, 256) {
				t.Fatalf("actual %s constructor value did not prove F32", path.access.fields[0].Name())
			}
			if ps6140ConcreteValue(unknownBackend, store.Val, linear, 256) {
				t.Fatal("unknown backend inherited selected F32 constructor proof")
			}
			if ps6140ConcreteValue(context, store.Val, quantized, 256) || ps6140ConcreteValue(context, store.Val, linear, 0) {
				t.Fatal("wrong concrete identity or exhausted proof budget accepted")
			}
			count++
		}
	}
	if count != 2 {
		t.Fatalf("actual projection assignments=%d, want2", count)
	}
	if appends != 1 {
		t.Fatalf("actual block append values=%d, want1", appends)
	}
}

func TestPS6140AuthenticRuntimeBlockMember(t *testing.T) {
	t.Parallel()
	fixture, err := ps6140CompileOwner(t, "before")
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	ownerType := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
	blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
	blocks := ps6136FieldVar(ownerType, "blocks")
	object, _, _ := types.LookupFieldOrMethod(ownerType, true, pkg.Pkg, "encodeStep")
	function := pkg.Prog.FuncValue(object.(*types.Func))
	context := ps6125NewSSAContext(function, nil, nil, 512)
	owner := context.reference(function.Params[0])
	count := 0
	for _, call := range ps6125ContextCalls(context) {
		callee := call.Call.StaticCallee()
		if callee == nil {
			continue
		}
		object, ok := callee.Object().(*types.Func)
		if !ok || object.Name() != "recordOProj" {
			continue
		}
		if len(call.Call.Args) != 4 || !ps6140BlockMember(context, call.Call.Args[2], owner, ownerType, blockType, blocks, 32) {
			ref := context.reference(call.Call.Args[2])
			t.Fatalf("actual runtime projection block not bound to sameowner collection: arg=%T %v reference=%T %v address=%T %v", call.Call.Args[2], call.Call.Args[2], ref.value, ref.value, call.Call.Args[2].(*ssa.UnOp).X, call.Call.Args[2].(*ssa.UnOp).X)
		}
		if ps6140BlockMember(context, call.Call.Args[2], ps6125SSAReference{}, ownerType, blockType, blocks, 32) {
			t.Fatal("unknownowner accepted")
		}
		count++
	}
	if count != 1 {
		t.Fatalf("recordOProj calls=%d want1", count)
	}
}

func TestPS6140AuthenticOwnerUnusedFormalBoundary(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileOwner(t, revision)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, file := range fixture.files {
				for _, declaration := range file.Decls {
					function, ok := declaration.(*ast.FuncDecl)
					if !ok || function.Recv == nil || function.Name.Name != "recordAdd" {
						continue
					}
					receiver, ok := function.Recv.List[0].Type.(*ast.Ident)
					if !ok || receiver.Name != "f32Linear" && receiver.Name != "quantLinear" {
						continue
					}
					// r is field0; grouped x,scratch,dst are field1 names.
					scratch := function.Type.Params.List[1].Names[1]
					object := fixture.info.Defs[scratch]
					uses := 0
					ast.Inspect(function.Body, func(node ast.Node) bool {
						if id, ok := node.(*ast.Ident); ok && fixture.info.Uses[id] == object {
							uses++
						}
						return true
					})
					if object == nil || receiver.Name == "f32Linear" && uses != 0 || receiver.Name == "quantLinear" && uses == 0 {
						t.Fatalf("%s scratch=%s typed uses=%d", receiver.Name, scratch.Name, uses)
					}
					found[receiver.Name] = true
				}
			}
			if !found["f32Linear"] || !found["quantLinear"] {
				t.Fatalf("missing authentic implementation: %v", found)
			}
		})
	}
}
