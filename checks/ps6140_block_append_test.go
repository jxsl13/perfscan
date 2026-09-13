package checks

import (
	"go/types"
	"testing"
)

func TestPS6140BlockAppendBoundaries(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type api interface{run()};type f32 struct{};func(f32)run(){};type quant struct{};func(quant)run(){}
type block struct{projection api};type owner struct{blocks []block}
func expose([]block){}
func same(d *owner){d.blocks=append(d.blocks,block{projection:f32{}})}
func wrong(d *owner){d.blocks=append(d.blocks,block{projection:quant{}})}
func multiple(d *owner){d.blocks=append(d.blocks,block{projection:f32{}},block{projection:f32{}})}
func borrowed(d *owner,input []block){d.blocks=append(d.blocks,input...)}
func escaped(d *owner){a:=[1]block{{projection:f32{}}};s:=a[:];expose(s);d.blocks=append(d.blocks,s...)}
`)
	blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
	concrete := pkg.Pkg.Scope().Lookup("f32").Type().(*types.Named)
	field := ps6136FieldVar(blockType, "projection")
	for _, name := range []string{"same", "wrong", "multiple", "borrowed", "escaped"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			context := ps6125NewSSAContext(pkg.Func(name), nil, nil, 32)
			found := false
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != nil {
					continue
				}
				appended := ps6140AppendedBlock(context, call, blockType)
				known := appended.value != nil && ps6140BlockProjectionValue(appended, field, concrete, 16)
				if known != (name == "same") {
					t.Fatalf("%s knownappend=%v", name, known)
				}
				found = true
			}
			if !found {
				t.Fatal("typed append absent")
			}
		})
	}
}
