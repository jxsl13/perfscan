package checks

import (
	"go/types"
	"testing"
)

func TestPS6140BlockProjectionSiblingWrites(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		want       bool
	}{
		{"literal", `b:=block{projection:quant{}};leaf(b)`, true},
		{"conditional-sibling", `b:=block{projection:quant{}};if flag{b.sibling=1};leaf(b)`, true},
		{"sibling-read-write", `b:=block{projection:quant{}};if b.sibling==0{b.sibling=2};leaf(b)`, true},
		{"selected-overwrite", `b:=block{projection:quant{}};if flag{b.projection=f32{}};leaf(b)`, false},
		{"conditional-selected", `var b block;if flag{b.projection=quant{}};leaf(b)`, false},
		{"whole-overwrite", `b:=block{projection:quant{}};if flag{b=block{projection:f32{}}};leaf(b)`, false},
		{"escaped-whole", `b:=block{projection:quant{}};if flag{b.sibling=1};escape(&b);leaf(b)`, false},
		{"escaped-sibling", `b:=block{projection:quant{}};escapeSibling(&b.sibling);leaf(b)`, false},
		{"captured-write", `b:=block{projection:quant{}};f:=func(){b.projection=f32{}};f();leaf(b)`, false},
		{"late-selected", `var b block;if flag{b.sibling=1};leaf(b);b.projection=quant{}`, false},
		{"opaque-value", `b:=opaque();if flag{b.sibling=1};leaf(b)`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type api interface{run()};type quant struct{};func(quant)run(){};type f32 struct{};func(f32)run(){}
type block struct{projection api;sibling int};func leaf(block){};func escape(*block);func escapeSibling(*int);func opaque()block
func entry(flag bool){`+test.body+`}`)
			context := ps6125NewSSAContext(pkg.Func("entry"), nil, nil, 256)
			block := pkg.Pkg.Scope().Lookup("block").Type().(*types.Named)
			concrete := pkg.Pkg.Scope().Lookup("quant").Type().(*types.Named)
			found := 0
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("leaf") {
					continue
				}
				found++
				payload := ps6125SSAReference{context: context, value: call.Call.Args[0]}
				if got := ps6140BlockProjectionValue(payload, ps6136FieldVar(block, "projection"), concrete, 256); got != test.want {
					t.Fatalf("closed selected projection=%v want=%v", got, test.want)
				}
				if ps6140BlockProjectionValue(payload, ps6136FieldVar(block, "projection"), concrete, 0) {
					t.Fatal("exhausted budget admitted")
				}
			}
			if found != 1 {
				t.Fatalf("actual typed block use count=%d want=1", found)
			}
		})
	}
}
