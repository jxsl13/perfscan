package checks

import (
	"go/types"
	"testing"
)

func TestPS6136HeadWidthSourceIdentity(t *testing.T) {
	t.Parallel()
	pkg := ps6125TestSSA(t, `package extent
type tensor struct{}
func(*tensor)Shape()[]int{return nil}
func(*tensor)Other()[]int{return nil}
type model struct{head,other *tensor}
func width(t *tensor)int{return t.Shape()[1]}
func leaf(int){}
func work(m,sibling *model){
 leaf(width(m.head));leaf(m.head.Shape()[1]);leaf(m.head.Shape()[0])
 leaf(sibling.head.Shape()[1]);leaf(m.other.Shape()[1]);leaf(m.head.Other()[1])
}
`)
	entry := pkg.Func("work")
	context := ps6125NewSSAContext(entry, nil, nil, 128)
	modelType := pkg.Pkg.Scope().Lookup("model").Type().(*types.Named)
	head := modelType.Underlying().(*types.Struct).Field(0)
	tensorType := pkg.Pkg.Scope().Lookup("tensor").Type().(*types.Named)
	method, _, _ := types.LookupFieldOrMethod(tensorType, true, pkg.Pkg, "Shape")
	model := context.reference(entry.Params[0])
	count := 0
	for _, call := range ps6125ContextCalls(context) {
		if call.Call.StaticCallee() != pkg.Func("leaf") {
			continue
		}
		got := ps6136HeadWidth(context, call.Call.Args[0], model, modelType, head, ps6090FunctionID(method.(*types.Func)), 1, 64)
		if got != (count < 2) {
			t.Fatalf("head witness %d = %v", count, got)
		}
		if ps6136HeadWidth(context, call.Call.Args[0], model, modelType, head, ps6090FunctionID(method.(*types.Func)), 1, 0) {
			t.Fatal("exhausted proof budget accepted")
		}
		count++
	}
	if count != 6 {
		t.Fatalf("leaf count %d", count)
	}
}
