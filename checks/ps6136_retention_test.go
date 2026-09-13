package checks

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

func TestPS6136FactoryRetentionPublicationOrder(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, retention string
		want            bool
	}{
		{"canonical", "d.all=append(d.all,b)", true},
		{"conditional-retention", "if retain{d.all=append(d.all,b)}", false},
		{"conditional-retention-else", "if retain{}else{d.all=append(d.all,b)}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type buffer interface{Release()};type slot struct{b buffer}
type ops struct{newBuffer func([]float32)(buffer,error)}
type owner struct{all []buffer;ops ops}
func mk(d *owner,data []float32,retain,blocked bool)*slot{
if blocked{return &slot{}};b,e:=d.ops.newBuffer(data);if e!=nil{return &slot{}};`+test.retention+`;return &slot{b:b}}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			list := ps6136FieldVar(ownerType, "all")
			allocator := ps6136FieldVar(pkg.Pkg.Scope().Lookup("ops").Type(), "newBuffer")
			slot := ps6136FieldVar(pkg.Pkg.Scope().Lookup("slot").Type(), "b")
			context := ps6125NewSSAContext(pkg.Func("mk"), nil, nil, 16384)
			owner := context.reference(context.flow.function.Params[0])
			input := context.reference(context.flow.function.Params[1])
			backend := ps6136FactoryResult(context, input, allocator, slot)
			if backend == nil || owner.value == nil || backend.Call.Signature().Results().Len() != 2 {
				t.Fatal("exact source backend/result/owner prerequisites missing")
			}
			var successfulReturns int
			for _, block := range context.flow.function.Blocks {
				for _, instruction := range block.Instrs {
					returned, ok := instruction.(*ssa.Return)
					if !ok {
						continue
					}
					_, fields, known := context.returnedFields(returned, 0)
					if known && fields[slot].value != nil {
						successfulReturns++
					}
				}
			}
			if successfulReturns == 0 {
				t.Fatal("non-vacuous successful slot publication missing")
			}
			if got := ps6136FactoryRetention(context, backend, owner, ownerType, list, slot); got != test.want {
				t.Fatalf("retention before successful slot publication=%v want=%v", got, test.want)
			}
		})
	}
}
