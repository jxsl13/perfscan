package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func ps6125TestSSA(t *testing.T, source string) *ssa.Package {
	t.Helper()
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "extent.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(&types.Config{}, files, types.NewPackage("extent", "extent"), []*ast.File{file}, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestPS6125SSARowSpecialization(t *testing.T) {
	t.Parallel()
	const source = `package extent
func produce(rows int) {}
func consume(elements int) {}
func step(tokens []int, width int) { produce(1); consume(width) }
func bulk(tokens []int, width int) { helper(tokens,width,false) }
func last(tokens []int, width int) { helper(tokens,width,true) }
func relay(tokens []int, width int) { last(tokens,width) }
func helper(tokens []int, width int, lastOnly bool) {
 k := len(tokens)
 if k == 0 { return }
 for _, token := range tokens { if token < 0 { return } }
 rows := k
 if lastOnly { rows = 1 }
 produce(rows)
 consume(rows*width)
}
func unknown(tokens []int, width int, lastOnly bool) { helper(tokens,width,lastOnly) }
func same(tokens []int, width int, choose bool) {
 rows := 1
 if choose { rows = 1 }
 produce(rows); consume(rows*width)
}
func overwrite(flag *bool) { *flag = false }
func addressed(tokens []int, width int) {
 lastOnly := true
 overwrite(&lastOnly)
 helper(tokens,width,lastOnly)
}
func captured(tokens []int, width int) {
 lastOnly := true
 change := func(){ lastOnly = false }
 change()
 helper(tokens,width,lastOnly)
}
func dead(tokens []int, width int) { if false { helper(tokens,width,true) } }
func ignoredClosure(tokens []int, width int) { _ = func(){ helper(tokens,width,true) } }
func grows(tokens []int, width int, choose bool) {
 rows := 1
 for choose { rows = rows*2 }
 produce(rows); consume(rows*width)
}
func flips(tokens []int, width int, choose bool) {
 lastOnly := true
 for choose { lastOnly = !lastOnly }
 helper(tokens,width,lastOnly)
}
`
	for _, test := range []struct {
		name string
		want string
	}{
		{"step", "one"}, {"last", "one"}, {"relay", "one"}, {"bulk", "bulk"},
		{"unknown", "unknown"}, {"same", "one"}, {"addressed", "unknown"},
		{"captured", "unknown"}, {"grows", "unknown"}, {"flips", "unknown"},
		{"dead", "none"}, {"ignoredClosure", "none"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pkg := ps6125TestSSA(t, source)
			function := pkg.Func(test.name)
			var pool ps6125ExtentPool
			rows := ps6125SymbolicExtent(pool.identity(function.Params[0].Object(), nil, true))
			width := ps6125SymbolicExtent(pool.identity(function.Params[1].Object(), nil, false))
			inputs := map[*ssa.Parameter]ps6125Scalar{function.Params[1]: ps6125IntegerFact(width)}
			lengths := map[*ssa.Parameter]ps6125Extent{function.Params[0]: rows}
			type invocation struct {
				function *ssa.Function
				inputs   map[*ssa.Parameter]ps6125Scalar
				lengths  map[*ssa.Parameter]ps6125Extent
			}
			queue := []invocation{{function, inputs, lengths}}
			var producers, consumers []ps6125Scalar
			// The fixture's call graph is acyclic. Recursive/multi-context package
			// traversal is not implemented or claimed by this local-flow test.
			for len(queue) != 0 {
				current := queue[0]
				queue = queue[1:]
				flow := ps6125AnalyzeSSAExtents(current.function, current.inputs, current.lengths)
				for _, block := range current.function.Blocks {
					for _, instruction := range block.Instrs {
						call, ok := instruction.(*ssa.Call)
						if !ok || !flow.blocks[block] {
							continue
						}
						callee := call.Call.StaticCallee()
						if callee == nil {
							continue
						}
						if callee.Name() == "produce" {
							producers = append(producers, flow.scalar(call.Call.Args[0]))
						} else if callee.Name() == "consume" {
							consumers = append(consumers, flow.scalar(call.Call.Args[0]))
						} else if next, arguments, sizes := flow.callInputs(call); next != nil {
							queue = append(queue, invocation{next, arguments, sizes})
						}
					}
				}
			}
			if test.want == "none" {
				if len(producers) != 0 || len(consumers) != 0 {
					t.Fatal("unexecuted body became producer/consumer evidence")
				}
				return
			}
			if len(producers) != 1 || len(consumers) != 1 {
				t.Fatalf("producer/consumer counts %d/%d", len(producers), len(consumers))
			}
			if test.want == "unknown" {
				if producers[0].state != ps6125Unknown || consumers[0].state != ps6125Unknown {
					t.Fatalf("unknown or mixed row extent became known: %v/%v", producers[0].state, consumers[0].state)
				}
				return
			}
			wantRows := ps6125ConstantExtent(1)
			if test.want == "bulk" {
				wantRows = rows
			}
			if producers[0].state != ps6125Integer || !ps6125SameExtent(producers[0].extent, wantRows) {
				t.Fatalf("wrong producer extent: %+v", producers[0])
			}
			if consumers[0].state != ps6125Integer || !ps6125SameExtent(consumers[0].extent, ps6125MultiplyExtents(wantRows, width)) {
				t.Fatalf("wrong consumer extent: %+v", consumers[0])
			}
		})
	}
}
