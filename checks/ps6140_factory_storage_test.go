package checks

import (
	"go/types"
	"testing"
)

func TestPS6140FactoryStorage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, factory, callback, wrapper string
		want                             bool
	}{
		{"genuine", "", "", "", true},
		{"factory-host-global", "host=data", "", "", false},
		{"factory-host-source", "keep(data)", "", "", false},
		{"factory-host-read", "count=len(data)", "", "", false},
		{"factory-host-capture", "saved=reader(data)", "", "", false},
		{"callback-host-global", "", "host=data", "", false},
		{"callback-host-source", "", "keep(data)", "", false},
		{"callback-host-read", "", "count=len(data)", "", false},
		{"wrapper-global", "", "", "wrapped=w", false},
		{"wrapper-boxed-global", "", "", "boxed=w", false},
		{"wrapper-source-observer", "", "", "observe(w)", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, pkg := ps6136CellTestPackage(t, `package cells
type buffer interface{Release()};type device struct{};func(*device)Release(){}
type wrapper struct{*device};type slot struct{b buffer}
type ops struct{newBuffer func([]float32)(buffer,error)}
var host []float32;var count int;var saved func();var wrapped wrapper;var boxed buffer
func keep(data []float32){host=data};func observe(w wrapper){wrapped=w}
func reader(data []float32)func(){return func(){keep(data)}}
// This source stub is only a typed leaf identity, never a native fact.
func native(data []float32)(*device,error){return nil,nil}
func New(n int)*slot{
o:=ops{newBuffer:func(data []float32)(buffer,error){`+test.callback+`;b,e:=native(data);if e!=nil{return nil,e};w:=wrapper{b};`+test.wrapper+`;return w,nil}}
var err error
mk:=func(data []float32)*slot{if err!=nil{return &slot{}};`+test.factory+`;b,e:=o.newBuffer(data);if e!=nil{err=e;return &slot{}};return &slot{b:b}}
return mk(make([]float32,n))}
`)
			context := ps6125NewSSAContext(pkg.Func("New"), nil, nil, 16384)
			slot := ps6136FieldVar(pkg.Pkg.Scope().Lookup("slot").Type(), "b")
			allocator := ps6136FieldVar(pkg.Pkg.Scope().Lookup("ops").Type(), "newBuffer")
			identities := map[string]bool{ps6090FunctionID(pkg.Func("native").Object().(*types.Func)): true}
			var factory *ps6125SSAContext
			for _, call := range ps6125ContextCalls(context) {
				if types.Identical(call.Type(), types.NewPointer(pkg.Pkg.Scope().Lookup("slot").Type())) {
					factory = ps6136Call(context, call)
				}
			}
			if factory == nil || len(factory.flow.function.Params) != 1 {
				t.Fatal("actual factory prerequisite missing")
			}
			input := factory.reference(factory.flow.function.Params[0])
			backend := ps6136FactoryResult(factory, input, allocator, slot)
			if backend == nil || !ps6136BoundAllocator(factory, backend, input, identities) {
				t.Fatal("genuine source result and exact typed native binding prerequisite missing")
			}
			proof := ps6140FactoryStorage(factory, allocator, slot, identities, 65536)
			if (proof != nil) != test.want {
				t.Fatalf("factory storage closure=%v want=%v", proof != nil, test.want)
			}
			if proof != nil && (proof.factory != factory || proof.backend != backend || proof.callback != factory.call(backend) || proof.native == nil || proof.wrapper != pkg.Pkg.Scope().Lookup("wrapper").Type()) {
				t.Fatal("factory/native/wrapper identity receipt changed")
			}
			if ps6140FactoryStorage(factory, allocator, slot, identities, 0) != nil || ps6140FactoryStorage(factory, allocator, slot, map[string]bool{"wrong.native": true}, 65536) != nil {
				t.Fatal("exhausted budget or wrong native leaf admitted")
			}
		})
	}
}
