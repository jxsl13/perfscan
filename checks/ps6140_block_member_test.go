package checks

import (
	"go/types"
	"testing"
)

func TestPS6140BlockMemberBoundaries(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type block struct{projection int};type owner struct{blocks []block;alternate []block}
func leaf(*owner,block){};func expose(*block){}
func same(d *owner){for _,b:=range d.blocks{leaf(d,b)}}
func wrong(d,other *owner){for _,b:=range other.blocks{leaf(d,b)}}
func alternate(d *owner){for _,b:=range d.alternate{leaf(d,b)}}
func mutated(d *owner){for _,b:=range d.blocks{b.projection++;leaf(d,b)}}
func escaped(d *owner){for _,b:=range d.blocks{expose(&b);leaf(d,b)}}
func rebound(d,other *owner){for _,b:=range d.blocks{d=other;leaf(d,b)}}
`)
	ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
	blockType := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
	blocks := ps6136FieldVar(ownerType, "blocks")
	for _, name := range []string{"same", "wrong", "alternate", "mutated", "escaped", "rebound"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			context := ps6125NewSSAContext(pkg.Func(name), nil, nil, 32)
			found := false
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("leaf") {
					continue
				}
				found = true
				owner := context.reference(call.Call.Args[0])
				if got := ps6140BlockMember(context, call.Call.Args[1], owner, ownerType, blockType, blocks, 16); got != (name == "same") {
					t.Fatalf("%s member=%v", name, got)
				}
			}
			if !found {
				t.Fatal("typed leaf absent")
			}
		})
	}
}
