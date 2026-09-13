package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6140ConstructorCallbacks(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, extra string
		want        bool
	}{
		{"genuine", "", true},
		{"invoked-returned", "reader(d)()", true},
		{"retained-returned", "leaked=reader(d)", false},
		{"retained-direct", "leaked=func(){sink=d.scratch}", false},
		{"source-retention", "keep(reader(d))", false},
		{"deferred-reader", "defer reader(d)()", false},
		{"asynchronous-reader", "go reader(d)()", false},
		{"forwarded-returned", "forward(reader(d))()", true},
		{"forwarded-retained", "leaked=forward(reader(d))", false},
		{"nested-private-cell", "fn:=reader(d);func(){fn()}()", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type owner struct{scratch int;marker bool};var sink int;var leaked func()
func reader(d *owner)func(){return func(){sink=d.scratch}}
func forward(fn func())func(){return fn};func keep(fn func()){leaked=fn}
func New()*owner{d:=&owner{scratch:1};`+test.extra+`;return d}
`)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			function := pkg.Func("New")
			context := ps6125NewSSAContext(function, nil, nil, 16384)
			var root ps6125SSAReference
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					if allocation, ok := instruction.(*ssa.Alloc); ok && types.Identical(allocation.Type(), types.NewPointer(owner)) {
						if root.value != nil {
							t.Fatal("fixture has multiple owner allocations")
						}
						root = context.reference(allocation)
					}
				}
			}
			if _, fresh := root.value.(*ssa.Alloc); !fresh {
				t.Fatal("actual fresh constructor prerequisite missing")
			}
			workspace := ps6136FieldVar(owner, "scratch")
			if got := ps6140ConstructorCallbacks(context, owner, workspace, 65536); got != test.want {
				t.Fatalf("constructor callback closure=%v want=%v", got, test.want)
			}
			if ps6140ConstructorCallbacks(context, owner, workspace, 1) || ps6140ConstructorCallbacks(nil, owner, workspace, 65536) {
				t.Fatal("missing constructor or exhausted work admitted")
			}
		})
	}
}
