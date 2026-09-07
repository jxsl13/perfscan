package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6083(t *testing.T) {
	t.Parallel()
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS6083.Analyzer, "ps6083")
}

func TestPS6083FixedPoint(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6083.Analyzer, "ps6083fixed")
}

func TestPS6083FixedTargetTypeCheck(t *testing.T) {
	t.Parallel()
	path := filepath.Join(analysistest.TestData(), "src", "ps6083", "ps6083.go.golden")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, path, source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	configuration := types.Config{}
	if _, err := configuration.Check("ps6083", files, []*ast.File{file}, nil); err != nil {
		t.Fatalf("PS6083 fixed target does not type-check: %v", err)
	}
}

func TestPS6083ExhaustiveBitEquality(t *testing.T) {
	t.Parallel()
	table32 := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
	for field := uint8(0); field < 8; field++ {
		original := float32(2*field + 1)
		if math.Float32bits(original) != math.Float32bits(table32[field]) {
			t.Fatalf("float32 field %d: original %#x, table %#x", field, math.Float32bits(original), math.Float32bits(table32[field]))
		}
	}
	table64 := [4]float64{2, 5, 8, 11}
	for field := uint16(0); field < 4; field++ {
		original := float64(3*field + 2)
		if math.Float64bits(original) != math.Float64bits(table64[field]) {
			t.Fatalf("float64 field %d: original %#x, table %#x", field, math.Float64bits(original), math.Float64bits(table64[field]))
		}
	}
}

func TestPS6083PackedSourceRandomizedEquivalence(t *testing.T) {
	t.Parallel()
	random := rand.New(rand.NewSource(809))
	table := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
	for sample := 0; sample < 10_000; sample++ {
		packed := random.Uint32()
		for index := range 8 {
			field := (packed >> (3 * index)) & 7
			original := float32(2*field + 1)
			if math.Float32bits(original) != math.Float32bits(table[field]) {
				t.Fatalf("sample %d index %d field %d: original %#x, table %#x", sample, index, field, math.Float32bits(original), math.Float32bits(table[field]))
			}
		}
	}
}

func TestPS6083ZeroTripPreservesSourceEvaluation(t *testing.T) {
	t.Parallel()
	beforeCalls, afterCalls := 0, 0
	beforeSource := func() uint32 {
		beforeCalls++
		return 7
	}
	afterSource := func() uint32 {
		afterCalls++
		return 7
	}
	before := func(output []float32) {
		for index := range output {
			output[index] = float32(2*(beforeSource()&7) + 1)
		}
	}
	after := func(output []float32) {
		lookup := [8]float32{1, 3, 5, 7, 9, 11, 13, 15}
		for index := range output {
			output[index] = lookup[afterSource()&7]
		}
	}
	before(nil)
	after(nil)
	if beforeCalls != 0 || afterCalls != 0 {
		t.Fatalf("zero-trip loop evaluated packed source: before=%d after=%d", beforeCalls, afterCalls)
	}
}
