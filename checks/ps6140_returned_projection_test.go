package checks

import (
	"go/types"
	"testing"
)

func TestPS6140ReturnedProjectionCapture(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, build, body string
		want              bool
	}{
		{"zero", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{});leaf(f())`, true},
		{"explicit-zero", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{fn:nil,flag:false});leaf(f())`, true},
		{"forwarded-invocation", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `leaf(invoke(build(ops{})))`, true},
		{"nonzero-function", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{fn:func(){}});leaf(f())`, false},
		{"nonzero-bool", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{flag:true});leaf(f())`, false},
		{"unknown", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(input);leaf(f())`, false},
		{"different-invocation", `return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `first:=build(ops{});_ = first();second:=build(ops{flag:true});leaf(second())`, false},
		{"captured-write", `return func()api{o.flag=true;if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{});leaf(f())`, false},
		{"later-write", `f:=func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}};o.flag=true;return f`, `f:=build(ops{});leaf(f())`, false},
		{"escaped-cell", `escape(&o);return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{});leaf(f())`, false},
		{"sibling-capture-write", `mutate:=func(){o.flag=true};mutate();return func()api{if o.fn==nil&&!o.flag{return f32{}};return quant{}}`, `f:=build(ops{});leaf(f())`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, `package extent
type api interface{run()};type f32 struct{};func(f32)run(){};type quant struct{};func(quant)run(){}
type ops struct{fn func();flag bool};func escape(*ops);func leaf(api){}
func invoke(f func()api)api{return f()}
func build(o ops)func()api{`+test.build+`}
func entry(input ops){`+test.body+`}`)
			context := ps6125NewSSAContext(pkg.Func("entry"), nil, nil, 4096)
			concrete := pkg.Pkg.Scope().Lookup("f32").Type().(*types.Named)
			found := 0
			for _, call := range ps6125ContextCalls(context) {
				if call.Call.StaticCallee() != pkg.Func("leaf") {
					continue
				}
				found++
				if got := ps6140ConcreteValue(context, call.Call.Args[0], concrete, 512); got != test.want {
					t.Fatalf("returned-capture dispatch=%v want=%v", got, test.want)
				}
				if ps6140ConcreteValue(context, call.Call.Args[0], concrete, 0) {
					t.Fatal("exhausted proof budget admitted")
				}
			}
			if found != 1 {
				t.Fatalf("actual typed consumer count=%d want=1", found)
			}
		})
	}
}
