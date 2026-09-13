package checks

import (
	"go/types"
	"testing"
)

func TestPS6140FactoryRetentionClosure(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, retention string
		want            bool
	}{
		{"canonical", "d.all=append(d.all,b)", true},
		{"conditional-retention", "if retain{d.all=append(d.all,b)}", false},
		{"conditional-retention-else", "if retain{}else{d.all=append(d.all,b)}", false},
		{"list-length", "count=len(d.all);d.all=append(d.all,b)", false},
		{"list-alias", "all=d.all;d.all=append(d.all,b)", false},
		{"append-result-alias", "next:=append(d.all,b);d.all=next;all=next", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type buffer interface{Release()};type slot struct{b buffer};type ops struct{newBuffer func([]float32)(buffer,error)}
type owner struct{all []buffer;ops ops};var all []buffer;var count int
func mk(d *owner,data []float32,retain,blocked bool)*slot{
if blocked{return &slot{}};b,e:=d.ops.newBuffer(data);if e!=nil{return &slot{}};`+test.retention+`;return &slot{b:b}}
`)
			ownerType := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			list := ps6136FieldVar(ownerType, "all")
			allocator := ps6136FieldVar(pkg.Pkg.Scope().Lookup("ops").Type(), "newBuffer")
			slot := ps6136FieldVar(pkg.Pkg.Scope().Lookup("slot").Type(), "b")
			context := ps6125NewSSAContext(pkg.Func("mk"), nil, nil, 16384)
			owner := context.reference(context.flow.function.Params[0])
			backend := ps6136FactoryResult(context, context.reference(context.flow.function.Params[1]), allocator, slot)
			// FactoryResult proves the same backend result is successfully
			// returned. Conditional retention must not require the old shared
			// helper's false admission: its released-rule fix rejects it too.
			conditional := test.name == "conditional-retention" || test.name == "conditional-retention-else"
			if backend == nil || !conditional && !ps6136FactoryRetention(context, backend, owner, ownerType, list, slot) {
				t.Fatal("genuine backend/slot/retention-shape prerequisite missing")
			}
			proof := ps6140FactoryRetention(context, backend, owner, ownerType, list, slot, 16384)
			if (proof != nil) != test.want {
				t.Fatalf("executed and isolated retention=%v want=%v", proof != nil, test.want)
			}
			if ps6140FactoryRetention(context, backend, owner, ownerType, list, slot, 0) != nil {
				t.Fatal("exhausted retention budget admitted")
			}
		})
	}
}
