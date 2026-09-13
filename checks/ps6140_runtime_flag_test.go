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

func TestPS6140RuntimeFlagInvocation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, before, call string
		want               bool
	}{
		{"direct", "", "read(d)", true},
		{"helper-chain", "", "relay(d)", true},
		{"different-owner", "other:=build();", "read(other)", false},
		{"unknown-owner", "", "read(external)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pass, pkg := ps6136CellTestPackage(t, `package cells
type owner struct{moe bool}
func build()*owner{d:=&owner{};_ = beforePublication(d);d.moe=true;return d}
func beforePublication(d *owner)bool{return d.moe}
func read(d *owner)bool{return d.moe}
func relay(d *owner)bool{return read(d)}
func run(external *owner)bool{d:=build();_ = d.moe;`+test.before+`return `+test.call+`}
`)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			entry := ps6125NewSSAContext(pkg.Func("run"), nil, nil, 16384)
			var constructor *ps6125SSAContext
			for _, call := range ps6125ContextCalls(entry) {
				if call.Call.StaticCallee() == pkg.Func("build") && constructor == nil {
					constructor = ps6136Call(entry, call)
				}
			}
			root := ps6140TestConstructorOwner(t, constructor, owner)
			snapshot := ps6140ConstructorFlags(constructor, root, owner, map[*types.Var]bool{ps6136FieldVar(owner, "moe"): true}, 65536)
			proof := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
			if proof == nil || !snapshot.values[ps6136FieldVar(owner, "moe")] {
				t.Fatal("actual immutable true publication prerequisite missing")
			}
			reads, early := 0, 0
			budget := 16384
			if !ps6136WalkConsumerCalls(entry, func(current *ps6125SSAContext, call *ssa.Call) bool {
				callee := call.Call.StaticCallee()
				if callee != pkg.Func("read") && callee != pkg.Func("beforePublication") {
					return false
				}
				child := ps6136Call(current, call)
				for _, block := range callee.Blocks {
					for _, instruction := range block.Instrs {
						load, ok := instruction.(*ssa.UnOp)
						if !ok || load.Op != token.MUL || !types.Identical(load.Type(), types.Typ[types.Bool]) {
							continue
						}
						truth, known := ps6140RuntimeFlag(proof, child, load, owner)
						if callee == pkg.Func("beforePublication") {
							early++
							if known {
								t.Fatal("constructor-time read used final published flag")
							}
							continue
						}
						reads++
						if known != test.want || known && !truth {
							t.Fatalf("runtime true=%v known=%v wantKnown=%v", truth, known, test.want)
						}
						if _, known := ps6140RuntimeFlag(proof, entry, load, owner); known {
							t.Fatal("foreign call context accepted")
						}
					}
				}
				return true
			}, &budget) || reads != 1 || early == 0 {
				t.Fatalf("actual read inventory incomplete reads=%d early=%d", reads, early)
			}
		})
	}
}

func TestPS6140AuthenticRuntimeFlags(t *testing.T) {
	t.Parallel()
	// This harness is explicitly synthetic. Its constructor and projection
	// methods are the complete pinned originals, not a historical user call.
	fixture, err := ps6140CompileLoadedMetalRewrite(t, "before", func(fs *token.FileSet, files []*ast.File) []*ast.File {
		harness, err := parser.ParseFile(fs, "flag_harness.go", `package llamagpu
import "github.com/jxsl13/goai/nlp"
func flagHarness(m *nlp.Llama,ops backendOps,r recorder)error{
 d,err:=newDecoder(m,ops);if err!=nil{return err}
 return d.recordOProj(r,block{},1)
}`, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return append(files, harness)
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg := ps6136FixtureSSA(fixture)
	owner := pkg.Pkg.Scope().Lookup("Decoder").Type().(*types.Named)
	entry := ps6125NewSSAContext(pkg.Func("flagHarness"), nil, nil, 16384)
	var constructor, consumer *ps6125SSAContext
	for _, call := range ps6125ContextCalls(entry) {
		if target := call.Call.StaticCallee(); target != nil {
			switch target.Name() {
			case "newDecoder":
				constructor = ps6136Call(entry, call)
			case "recordOProj":
				consumer = ps6136Call(entry, call)
			}
		}
	}
	if constructor == nil || consumer == nil {
		t.Fatal("actual source constructor/projection invocation missing")
	}
	root := ps6140TestConstructorOwner(t, constructor, owner)
	fields := map[*types.Var]bool{ps6136FieldVar(owner, "postNorm"): true, ps6136FieldVar(owner, "sandwich"): true}
	snapshot := ps6140ConstructorFlags(constructor, root, owner, fields, 65536)
	pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
	proof := ps6140ImmutableConstructorFlags(pass, pkg, snapshot, owner, 65536)
	if proof == nil {
		t.Fatal("authentic immutable snapshot prerequisite missing")
	}
	reads := 0
	for _, block := range consumer.flow.function.Blocks {
		for _, instruction := range block.Instrs {
			load, ok := instruction.(*ssa.UnOp)
			if !ok || !types.Identical(load.Type(), types.Typ[types.Bool]) {
				continue
			}
			truth, known := ps6140RuntimeFlag(proof, consumer, load, owner)
			if !known || truth {
				t.Fatal("actual dense constructor flag was not joined to source projection read")
			}
			reads++
		}
	}
	if reads != 3 {
		t.Fatalf("actual postNorm/sandwich read census=%d want3", reads)
	}
}
