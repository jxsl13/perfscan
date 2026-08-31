package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"math"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6092(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6092.Analyzer, "ps6092")
}

func TestPS6092WrappedAdd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		value             int64
		step              int64
		minimum           int64
		maximum           int64
		want              int64
		wantRepresentable bool
	}{
		{name: "signed8 overflow", value: math.MaxInt8, step: 1, minimum: math.MinInt8, maximum: math.MaxInt8, want: math.MinInt8, wantRepresentable: true},
		{name: "signed8 underflow", value: math.MinInt8, step: -1, minimum: math.MinInt8, maximum: math.MaxInt8, want: math.MaxInt8, wantRepresentable: true},
		{name: "unsigned8 overflow", value: math.MaxUint8, step: 1, minimum: 0, maximum: math.MaxUint8, want: 0, wantRepresentable: true},
		{name: "unsigned8 underflow", value: 0, step: -1, minimum: 0, maximum: math.MaxUint8, want: math.MaxUint8, wantRepresentable: true},
		{name: "signed64 overflow", value: math.MaxInt64, step: 1, minimum: math.MinInt64, maximum: math.MaxInt64, want: math.MinInt64, wantRepresentable: true},
		{name: "signed64 underflow", value: math.MinInt64, step: -1, minimum: math.MinInt64, maximum: math.MaxInt64, want: math.MaxInt64, wantRepresentable: true},
		{name: "signed64 minimum step", value: 0, step: math.MinInt64, minimum: math.MinInt64, maximum: math.MaxInt64, want: math.MinInt64, wantRepresentable: true},
		{name: "signed8 large overflow", value: math.MaxInt8, step: math.MaxInt8, minimum: math.MinInt8, maximum: math.MaxInt8, want: -2, wantRepresentable: true},
		{name: "signed8 large underflow", value: math.MinInt8, step: -math.MaxInt8, minimum: math.MinInt8, maximum: math.MaxInt8, want: 1, wantRepresentable: true},
		{name: "unsigned8 large overflow", value: math.MaxUint8, step: math.MaxUint8, minimum: 0, maximum: math.MaxUint8, want: math.MaxUint8 - 1, wantRepresentable: true},
		{name: "unsigned8 large underflow", value: 0, step: -math.MaxUint8, minimum: 0, maximum: math.MaxUint8, want: 1, wantRepresentable: true},
		{name: "unsigned64 leaves simulator", value: math.MaxInt64, step: 1, minimum: 0, maximum: math.MaxInt64, wantRepresentable: false},
	}
	for _, test := range tests {
		got, representable := ps6092WrappedAdd(test.value, test.step, test.minimum, test.maximum)
		if got != test.want || representable != test.wantRepresentable {
			t.Errorf("%s: ps6092WrappedAdd(%d, %d, %d, %d) = (%d, %t), want (%d, %t)", test.name, test.value, test.step, test.minimum, test.maximum, got, representable, test.want, test.wantRepresentable)
		}
	}
}

func TestPS6092MapLiteralTripBoundLinear(t *testing.T) {
	t.Parallel()
	const entries = 512
	var source strings.Builder
	source.WriteString("package complexity\nfunc f(key int) { _ = map[int]bool{")
	for index := range entries {
		source.WriteString("key+")
		source.WriteString(strconv.Itoa(index))
		source.WriteString(":true,")
	}
	source.WriteString("key+0:false} }\n")

	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "complexity.go", source.String(), 0)
	if err != nil {
		t.Fatal(err)
	}
	info := types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	configuration := types.Config{Sizes: types.SizesFor("gc", "amd64")}
	if _, err := configuration.Check("complexity", files, []*ast.File{file}, &info); err != nil {
		t.Fatal(err)
	}
	var literal *ast.CompositeLit
	ast.Inspect(file, func(node ast.Node) bool {
		candidate, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if _, mapType := types.Unalias(info.TypeOf(candidate)).Underlying().(*types.Map); mapType {
			literal = candidate
			return false
		}
		return true
	})
	if literal == nil {
		t.Fatal("map literal not found")
	}
	bound, visited := ps6092MapLiteralTripBoundWork(&analysis.Pass{TypesInfo: &info}, literal)
	if bound != entries {
		t.Fatalf("bound = %d, want %d", bound, entries)
	}
	if maximum := 4 * (entries + 1); visited > maximum {
		t.Fatalf("visited %d expression nodes for %d keys, want at most %d", visited, entries+1, maximum)
	}
}
