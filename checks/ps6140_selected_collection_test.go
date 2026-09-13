package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestPS6140AuthenticSelectedCollection(t *testing.T) {
	t.Parallel()
	for _, revision := range []string{"before", "after"} {
		t.Run(revision, func(t *testing.T) {
			t.Parallel()
			fixture, err := ps6140CompileLoadedMetal(t, revision)
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
				t.Fatal("genuine constructor missing")
			}
			pass := &analysis.Pass{Fset: fixture.fileset, Files: fixture.files, TypesInfo: fixture.info, Pkg: fixture.pkg}
			projectors := map[*types.Var]*types.Named{ps6136FieldVar(block, "wo"): concrete, ps6136FieldVar(block, "wD"): concrete}
			proof := ps6140SelectedCollection(pass, pkg, constructor, owner, block, ps6136FieldVar(owner, "blocks"), projectors, 65536)
			if proof == nil {
				ps6140SelectedCollectionCheck(pass, pkg, constructor, owner, block, ps6136FieldVar(owner, "blocks"), projectors, 65536, func(stage string) { t.Logf("rejected %s", stage) })
				t.Fatal("genuine selected collection rejected")
			}
			if proof.constructor != constructor || proof.owner.value == nil || proof.appended.value == nil {
				t.Fatal("collection lifetime association absent")
			}
		})
	}
}

func TestPS6140PrivateLiteralCopyJoin(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"copied-literal", `gb:=block{projection:f32{}};if flag {gb.sibling=1};d.blocks=append(d.blocks,gb)`, true},
		{"escaped-cell", `gb:=block{projection:f32{}};if flag {gb.sibling=1};expose(&gb);d.blocks=append(d.blocks,gb)`, false},
		{"opaque-value", `gb:=opaque();if flag {gb.sibling=1};d.blocks=append(d.blocks,gb)`, false},
		{"write-after-copy", `gb:=block{projection:f32{}};if flag {gb.sibling=1};d.blocks=append(d.blocks,gb);gb.projection=f32{}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent;type api interface{run()};type f32 struct{};func(f32)run(){};type block struct{projection api;sibling int};type owner struct{blocks []block};func expose(*block){};func opaque()block;func build(d *owner,flag bool){`+test.body+`}`)
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			projection := ps6136FieldVar(block, "projection")
			context := ps6125NewSSAContext(pkg.Func("build"), nil, nil, 128)
			graph := &ps6140CollectionGraph{block: block, remaining: 1024}
			for _, call := range ps6125ContextCalls(context) {
				builtin, ok := call.Call.Value.(*ssa.Builtin)
				if !ok || builtin.Name() != "append" {
					continue
				}
				payload := ps6140AppendedBlock(context, call, block)
				positions := ps6140PrivateConstructedBlock(context, payload, call.Pos(), graph)
				if (positions != nil) != test.want {
					t.Fatalf("join=%v, want %v (%s)", positions != nil, test.want, graph.last)
				}
				if test.want {
					copied, literal := false, false
					for _, bb := range context.flow.function.Blocks {
						for _, instruction := range bb.Instrs {
							store, ok := instruction.(*ssa.Store)
							if !ok {
								continue
							}
							if _, ok := store.Val.(*ssa.UnOp); ok && types.Identical(store.Val.Type(), block) {
								copied = true
							}
							if field, ok := store.Addr.(*ssa.FieldAddr); ok {
								if pointer, ok := field.X.Type().Underlying().(*types.Pointer); ok && types.Identical(pointer.Elem(), block) && block.Underlying().(*types.Struct).Field(field.Field) == projection {
									literal = positions[store.Pos()]
								}
							}
						}
					}
					if !copied || !literal {
						t.Fatal("typed intermediate literal copy or exact projector initializer position absent")
					}
				}
				return
			}
			t.Fatal("typed append absent")
		})
	}
}
