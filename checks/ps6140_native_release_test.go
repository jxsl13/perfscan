package checks

import (
	"go/types"
	"testing"
)

func TestPS6140NativeReleaseBinding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, wrapper string
		want          bool
	}{
		{"promoted-native-pointer", "type wrapper struct{*device}", true},
		{"shadowed-noop", "type wrapper struct{*device};func(wrapper)Release(){}", false},
		{"explicit-wrapper-needs-body-proof", "type wrapper struct{*device};func(w wrapper)Release(){w.device.Release()}", false},
		{"unembedded-field", "type wrapper struct{d *device};func(w wrapper)Release(){w.d.Release()}", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type buffer interface{Release()};type device struct{};func(*device)Release(){}
`+test.wrapper+`
type slot struct{b buffer};type ops struct{newBuffer func([]float32)(buffer,error)}
// Typed source boundary only: this stub does not establish native semantics.
func native(data []float32)(*device,error){return nil,nil}
func New(n int)*slot{
o:=ops{newBuffer:func(data []float32)(buffer,error){b,e:=native(data);if e!=nil{return nil,e};return wrapper{b},nil}}
var err error
mk:=func(data []float32)*slot{if err!=nil{return &slot{}};b,e:=o.newBuffer(data);if e!=nil{err=e;return &slot{}};return &slot{b:b}}
return mk(make([]float32,n))}
`)
			context := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			slot := ps6136FieldVar(pkg.Pkg.Scope().Lookup("slot").Type(), "b")
			allocator := ps6136FieldVar(pkg.Pkg.Scope().Lookup("ops").Type(), "newBuffer")
			identities := map[string]bool{ps6090FunctionID(pkg.Func("native").Object().(*types.Func)): true}
			var factory *ps6125SSAContext
			for _, call := range ps6136Calls(context) {
				if types.Identical(call.Type(), types.NewPointer(pkg.Pkg.Scope().Lookup("slot").Type())) {
					factory = ps6136Call(context, call)
				}
			}
			storage := ps6140FactoryStorage(factory, allocator, slot, identities, 65536)
			if storage == nil {
				t.Fatal("genuine factory/result/wrapper exclusive-storage prerequisite missing")
			}
			retained, _, _ := types.LookupFieldOrMethod(slot.Type(), false, pkg.Pkg, "Release")
			native, _, _ := types.LookupFieldOrMethod(types.NewPointer(pkg.Pkg.Scope().Lookup("device").Type()), false, pkg.Pkg, "Release")
			identity := ps6090FunctionID(native.(*types.Func))
			bound := ps6140NativeReleaseBinding(storage, retained.(*types.Func), identity, 1024)
			if (bound != nil) != test.want || bound != nil && bound != native {
				t.Fatalf("exact native release promotion=%v want=%v", bound != nil, test.want)
			}
			if ps6140NativeReleaseBinding(storage, retained.(*types.Func), "other.Device.Release", 1024) != nil || ps6140NativeReleaseBinding(storage, retained.(*types.Func), identity, 0) != nil {
				t.Fatal("wrong native identity or exhausted budget admitted")
			}
		})
	}
}
