package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6125TypedSSAAccessPaths(t *testing.T) {
	t.Parallel()
	const source = `package extent
type buffer struct{ value int }
type slot struct{ b *buffer }
type decoder struct{ slot *slot; other *slot; slots []*slot }
func consume(*buffer) {}
func opaque() *decoder { return nil }
func direct(d, sibling *decoder, choose bool) {
 consume(d.slot.b)
 alias := d
 consume(alias.slot.b)
 consume(d.other.b)
 consume(sibling.slot.b)
 selected := d
 if choose { selected = sibling }
 consume(selected.slot.b)
 consume(opaque().slot.b)
 consume(d.slots[0].b)
}
func snapshots(d *decoder, choose bool) {
 first := d.slot.b
 second := d.slot.b
 selected := first
 if choose { selected = second }
 consume(selected)
}
func specialized(d, sibling *decoder, choose bool) {
 selected := d
 if choose { selected = sibling }
 consume(selected.slot.b)
}
func cyclic(d *decoder, choose bool) {
 selected := d
 for choose { selected = opaque() }
 consume(selected.slot.b)
}
`
	pkg := ps6125TestSSA(t, source)
	owner := pkg.Pkg.Scope().Lookup("decoder").Type().Underlying().(*types.Struct)
	slot := pkg.Pkg.Scope().Lookup("slot").Type().Underlying().(*types.Struct)
	for _, test := range []struct {
		name string
		flag *bool
		want []int // 0 unknown, 1 d.slot.b, 2 d.other.b, 3 sibling.slot.b
	}{
		{"direct", nil, []int{1, 1, 2, 3, 0, 0, 0}},
		{"snapshots", nil, []int{0}},
		{"specialized", ps6125TestBool(false), []int{1}},
		{"specialized", ps6125TestBool(true), []int{3}},
		{"cyclic", nil, []int{0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			function := pkg.Func(test.name)
			inputs := make(map[*ssa.Parameter]ps6125Scalar)
			if test.flag != nil {
				inputs[function.Params[len(function.Params)-1]] = ps6125Scalar{state: ps6125Boolean, truth: *test.flag}
			}
			flow := ps6125AnalyzeSSAExtents(function, inputs, nil)
			paths := &ps6125AccessPaths{flow: flow}
			found := 0
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					call, ok := instruction.(*ssa.Call)
					if !ok || !flow.blocks[block] || call.Call.StaticCallee() == nil || call.Call.StaticCallee().Name() != "consume" {
						continue
					}
					if found >= len(test.want) {
						t.Fatal("unexpected access observation")
					}
					result := paths.resolve(call.Call.Args[0])
					want := test.want[found]
					found++
					if want == 0 {
						if result.known {
							t.Fatal("unknown, mixed, cyclic or separately loaded origin was equated")
						}
						continue
					}
					root, field := function.Params[0], owner.Field(0)
					if want == 2 {
						field = owner.Field(1)
					}
					if want == 3 {
						root = function.Params[1]
					}
					if !result.known || result.access.root != root || len(result.access.fields) != 2 || result.access.fields[0] != field || result.access.fields[1] != slot.Field(0) || len(result.access.loads) != 2 {
						t.Fatalf("incorrect typed root/field/load description: %+v", result)
					}
				}
			}
			if found != len(test.want) {
				t.Fatalf("got %d observations, want %d", found, len(test.want))
			}
		})
	}
}

func ps6125TestBool(value bool) *bool { return &value }
