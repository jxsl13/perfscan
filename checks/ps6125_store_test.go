package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6125ReturnedFieldStores(t *testing.T) {
	t.Parallel()
	const source = `package extent
type cell struct{ width int }
var global *cell
var fieldGlobal *int
func touch(*cell) {}
func direct(width int) *cell { return &cell{width: width} }
func reused(d *cell) *cell { return d }
func escaped(width int) *cell { d := &cell{width: width}; global = d; return d }
func called(width int) *cell { d := &cell{width: width}; touch(d); return d }
func addressed(width int) *cell { d := &cell{width: width}; fieldGlobal = &d.width; return d }
func captured(width int) *cell { d := &cell{width: width}; _ = func(){ d.width = 0 }; return d }
func whole(width int) *cell { d := &cell{}; *d = cell{width: width}; return d }
func mixed(width int, choose bool) *cell {
 d := &cell{width: width}
 if choose { d.width = 0 }
 return d
}
func conditional(width int, choose bool) *cell {
 d := &cell{}
 if choose { d.width = width }
 return d
}
func loaded(width int) *cell { d := &cell{width: width}; _ = d.width; return d }
func zero() *cell { return &cell{} }
`
	pkg := ps6125TestSSA(t, source)
	field := pkg.Pkg.Scope().Lookup("cell").Type().Underlying().(*types.Struct).Field(0)
	for _, test := range []struct {
		name  string
		flag  *bool
		known bool
		value bool
	}{
		{"direct", nil, true, true},
		{"reused", nil, false, false},
		{"escaped", nil, false, false},
		{"called", nil, false, false},
		{"addressed", nil, false, false},
		{"captured", nil, false, false},
		{"whole", nil, false, false},
		{"mixed", nil, true, false},
		{"mixed", ps6125TestBool(false), true, true},
		{"conditional", nil, true, false},
		{"conditional", ps6125TestBool(true), true, true},
		{"loaded", nil, true, true},
		{"zero", nil, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			function := pkg.Func(test.name)
			inputs := make(map[*ssa.Parameter]ps6125Scalar)
			if test.flag != nil {
				inputs[function.Params[len(function.Params)-1]] = ps6125Scalar{state: ps6125Boolean, truth: *test.flag}
			}
			flow := ps6125AnalyzeSSAExtents(function, inputs, nil)
			origins := &ps6125SSAOrigins{flow: flow}
			found := 0
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					returned, ok := instruction.(*ssa.Return)
					if !ok || !flow.blocks[block] {
						continue
					}
					found++
					_, fields, known := origins.returnedFields(returned, 0)
					if known != test.known || (fields[field] != nil) != test.value {
						t.Fatalf("returned field known=%v fields=%v, want known=%v value=%v", known, fields, test.known, test.value)
					}
					if test.value && fields[field] != function.Params[0] {
						t.Fatal("field value was not the exact source argument")
					}
				}
			}
			if found != 1 {
				t.Fatalf("got %d returns, want 1", found)
			}
		})
	}
}
