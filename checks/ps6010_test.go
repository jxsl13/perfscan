package checks

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6010ValueAndReferenceStorageDisjointness(t *testing.T) {
	t.Parallel()

	float64Type := types.Typ[types.Float64]
	intType := types.Typ[types.Int]
	floatArray := types.NewArray(float64Type, 8)
	namedArray := types.NewNamed(types.NewTypeName(token.NoPos, nil, "namedArray", nil), floatArray, nil)
	nestedArray := types.NewArray(floatArray, 2)
	field := types.NewField(token.NoPos, nil, "values", floatArray, false)
	holder := types.NewStruct([]*types.Var{field}, nil)
	floatSlice := types.NewSlice(float64Type)
	intSlice := types.NewSlice(intType)

	for name, valueType := range map[string]types.Type{
		"array":         floatArray,
		"named array":   namedArray,
		"array element": nestedArray,
		"struct field":  holder,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			facts := &ps6010DefinitionFacts{}
			value := types.NewVar(token.NoPos, nil, "value", valueType)
			reference := types.NewVar(token.NoPos, nil, "reference", floatSlice)
			if facts.storageProvenDisjoint(value, reference) {
				t.Fatal("slice storage may point into addressable value storage")
			}
		})
	}

	facts := &ps6010DefinitionFacts{}
	floatValue := types.NewVar(token.NoPos, nil, "floats", floatArray)
	intReference := types.NewVar(token.NoPos, nil, "ints", intSlice)
	if !facts.storageProvenDisjoint(floatValue, intReference) {
		t.Fatal("[]int cannot point into [8]float64 storage in safe Go")
	}
	scalar := types.NewVar(token.NoPos, nil, "scalar", intType)
	floatReference := types.NewVar(token.NoPos, nil, "reference", floatSlice)
	if !facts.storageProvenDisjoint(scalar, floatReference) {
		t.Fatal("a slice cannot point into unrelated scalar storage in safe Go")
	}
	otherArray := types.NewVar(token.NoPos, nil, "other", floatArray)
	if !facts.storageProvenDisjoint(floatValue, otherArray) {
		t.Fatal("distinct value-only array objects do not share storage")
	}
}

func TestPS6010NestedReferenceAliasCompatibility(t *testing.T) {
	t.Parallel()

	float64Type := types.Typ[types.Float64]
	intType := types.Typ[types.Int]
	floatSlice := types.NewSlice(float64Type)
	leftLeaf := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "values", floatSlice, false),
	}, nil)
	rightLeaf := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "other", floatSlice, false),
	}, nil)
	if !ps6010ReferenceTypesMayAlias(leftLeaf, rightLeaf) {
		t.Fatal("distinct structs can retain slices sharing one backing array")
	}

	leftNested := types.NewArray(types.NewPointer(leftLeaf), 2)
	rightNested := types.NewArray(types.NewPointer(rightLeaf), 3)
	if !ps6010ReferenceTypesMayAlias(leftNested, rightNested) {
		t.Fatal("nested arrays and pointers must expose reference-bearing descendants")
	}

	leftMap := types.NewMap(intType, leftLeaf)
	rightMap := types.NewMap(types.Typ[types.String], rightLeaf)
	if !ps6010ReferenceTypesMayAlias(leftMap, rightMap) {
		t.Fatal("different maps may retain values whose slice descendants alias")
	}

	leftChannel := types.NewChan(types.SendRecv, leftLeaf)
	rightChannel := types.NewChan(types.RecvOnly, rightLeaf)
	if !ps6010ReferenceTypesMayAlias(leftChannel, rightChannel) {
		t.Fatal("channel payload descendants may retain aliased reference storage")
	}

	callback := types.NewSignatureType(nil, nil, nil,
		types.NewTuple(types.NewParam(token.NoPos, nil, "output", intType)),
		types.NewTuple(), false)
	if !ps6010ReferenceTypesMayAlias(callback, floatSlice) {
		t.Fatal("a callback may hide a capture of a reference-bearing parameter")
	}

	leftValue := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "values", types.NewArray(float64Type, 8), false),
	}, nil)
	rightValue := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "other", types.NewArray(float64Type, 8), false),
	}, nil)
	if ps6010ReferenceTypesMayAlias(leftValue, rightValue) {
		t.Fatal("separately copied value-only aggregates do not share storage")
	}
	if ps6010ReferenceTypesMayAlias(types.NewSlice(intType), floatSlice) {
		t.Fatal("slices with incompatible element types cannot share safe-Go backing storage")
	}
	if ps6010ReferenceTypesMayAlias(types.NewChan(types.SendRecv, intType), floatSlice) {
		t.Fatal("a value-only channel payload does not alias unrelated slice storage")
	}
	floatArrayPointer := types.NewPointer(types.NewArray(float64Type, 8))
	if !ps6010ReferenceTypesMayAlias(floatSlice, floatArrayPointer) ||
		!ps6010ReferenceTypesMayAlias(floatArrayPointer, floatSlice) {
		t.Fatal("a slice and pointer to a compatible array may address the same storage in either order")
	}
	namedArray := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "namedRound14Array", nil),
		types.NewArray(float64Type, 8), nil,
	)
	namedSlice := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "namedRound14Slice", nil),
		floatSlice, nil,
	)
	if !ps6010ReferenceTypesMayAlias(namedSlice, types.NewPointer(namedArray)) {
		t.Fatal("named slice and named array pointer may address the same storage")
	}
	if ps6010ReferenceTypesMayAlias(floatSlice, types.NewPointer(types.NewArray(intType, 8))) {
		t.Fatal("a float slice cannot address an int array in safe Go")
	}
	floatPointer := types.NewPointer(float64Type)
	if !ps6010ReferenceTypesMayAlias(floatSlice, floatPointer) ||
		!ps6010ReferenceTypesMayAlias(floatPointer, floatSlice) {
		t.Fatal("a slice and pointer to one of its elements may share storage in either order")
	}
	if !ps6010ReferenceTypesMayAlias(floatArrayPointer, floatPointer) ||
		!ps6010ReferenceTypesMayAlias(floatPointer, floatArrayPointer) {
		t.Fatal("whole-array and element pointers may share a subobject in either order")
	}
	if ps6010ReferenceTypesMayAlias(floatSlice, types.NewPointer(intType)) ||
		ps6010ReferenceTypesMayAlias(floatArrayPointer, types.NewPointer(intType)) {
		t.Fatal("incompatible element-pointer types cannot share safe-Go storage")
	}
	namedFloat := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "namedRound16Float", nil),
		float64Type, nil,
	)
	namedFloatSlice := types.NewSlice(namedFloat)
	if !ps6010ReferenceTypesMayAlias(namedFloatSlice, floatPointer) {
		t.Fatal("a permitted pointer conversion can expose []namedFloat storage as *float64")
	}
	namedFloatArrayPointer := types.NewPointer(types.NewArray(namedFloat, 8))
	if !ps6010ReferenceTypesMayAlias(namedFloatArrayPointer, floatPointer) {
		t.Fatal("a permitted pointer conversion can expose *[N]namedFloat storage as *float64")
	}
	if ps6010ReferenceTypesMayAlias(types.NewSlice(types.Typ[types.Int64]), floatPointer) {
		t.Fatal("incompatible scalar sizes cannot share safe-Go subobject storage")
	}
}

func TestPS6010StorageCompatibilityIndexConservative(t *testing.T) {
	t.Parallel()

	float64Type := types.Typ[types.Float64]
	intType := types.Typ[types.Int]
	namedFloat := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "indexedFloat", nil),
		float64Type, nil,
	)
	floatSlice := types.NewSlice(float64Type)
	namedFloatSlice := types.NewSlice(namedFloat)
	floatPointer := types.NewPointer(float64Type)
	floatArrayPointer := types.NewPointer(types.NewArray(float64Type, 8))
	leafA := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "a", floatSlice, false),
	}, nil)
	leafB := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, nil, "b", namedFloatSlice, false),
	}, nil)
	callback := types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(), false)
	iface := types.NewInterfaceType(nil, nil)
	iface.Complete()
	typesToCheck := []types.Type{
		floatSlice,
		namedFloatSlice,
		types.NewSlice(intType),
		floatPointer,
		floatArrayPointer,
		types.NewPointer(types.NewArray(namedFloat, 8)),
		types.NewPointer(intType),
		leafA,
		leafB,
		types.NewArray(types.NewPointer(leafA), 2),
		types.NewArray(types.NewPointer(leafB), 3),
		types.NewMap(intType, leafA),
		types.NewMap(types.Typ[types.String], leafB),
		types.NewChan(types.SendRecv, leafA),
		types.NewChan(types.RecvOnly, leafB),
		callback,
		iface,
	}
	classesMayMatch := func(left, right types.Type) bool {
		leftClasses, leftWildcard := ps6010StorageCompatibilityClasses(left)
		rightClasses, rightWildcard := ps6010StorageCompatibilityClasses(right)
		if leftWildcard || rightWildcard {
			return true
		}
		for class := range leftClasses {
			if rightClasses[class] {
				return true
			}
		}
		return false
	}
	for leftIndex, left := range typesToCheck {
		for rightIndex, right := range typesToCheck {
			if ps6010ReferenceTypesMayAlias(left, right) && !classesMayMatch(left, right) {
				t.Fatalf("indexed classes under-approximate conservative pair %d/%d: %s and %s",
					leftIndex, rightIndex, left, right)
			}
		}
	}
	if !classesMayMatch(namedFloatSlice, floatPointer) {
		t.Fatal("named scalar pointer conversions must retain a shared compatibility class")
	}
	if classesMayMatch(types.NewSlice(intType), floatPointer) {
		t.Fatal("incompatible int and float storage must remain in distinct compatibility classes")
	}
}

func TestPS6010AnalyzerDominanceScaling(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		var source strings.Builder
		source.WriteString("package p\nfunc scale(a [2048]float64, w [4]float64, out, n int) [4]float64 {\nvar dst [4]float64\n")
		for index := range size {
			fmt.Fprintf(&source, "base%d := 0\n", index)
		}
		for index := range size {
			fmt.Fprintf(&source, `for o%d := 0; o%d < out; o%d++ {
acc%d := 0.0
for i%d := 0; i%d < n; i%d++ {
acc%d += a[base%d+i%d] * w[o%d]
}
dst[o%d] = acc%d
}
`, index, index, index, index, index, index, index, index, index, index, index, index, index)
		}
		source.WriteString("return dst\n}\n")

		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "scale.go", source.String(), 0)
		if err != nil {
			t.Fatalf("parse size %d analyzer fixture: %v", size, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check size %d analyzer fixture: %v", size, err)
		}
		diagnostics := 0
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) { diagnostics++ },
		}
		stats := &ps6010RunStats{}
		if err := ps6010Run(pass, stats); err != nil {
			t.Fatalf("run size %d analyzer fixture: %v", size, err)
		}
		if diagnostics != size {
			t.Fatalf("size %d: got %d diagnostics, want %d", size, diagnostics, size)
		}
		if stats.dominanceQueries != size {
			t.Fatalf("size %d: got %d dominance queries, want %d", size, stats.dominanceQueries, size)
		}
		if stats.dominanceSteps > 3*size {
			t.Fatalf("size %d: dominance owner walk exceeded bounded nesting: %d", size, stats.dominanceSteps)
		}
		return stats.indexedStatements + stats.dominanceQueries + stats.dominanceSteps
	}

	var previous int
	for _, size := range []int{100, 200, 400, 800} {
		current := work(size)
		if current > 12*size+16 {
			t.Fatalf("size %d: real analyzer dominance work is not linear: %d", size, current)
		}
		if previous != 0 && current > 2*previous+16 {
			t.Fatalf("doubling real analyzer input grew indexed dominance work superlinearly: %d -> %d", previous, current)
		}
		previous = current
	}
}

func TestPS6010PointerProvenanceScaling(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		var source strings.Builder
		source.WriteString("package p\nfunc scale() {\nvar x int\n")
		for index := 0; index <= size; index++ {
			fmt.Fprintf(&source, "var p%d *int\n", index)
		}
		for index := 0; index < size; index++ {
			fmt.Fprintf(&source, "p%d = p%d\n", index, index+1)
		}
		fmt.Fprintf(&source, "p%d = &x\n_ = p0\n}\n", size)

		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "pointer_scale.go", source.String(), 0)
		if err != nil {
			t.Fatalf("parse size %d pointer fixture: %v", size, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check size %d pointer fixture: %v", size, err)
		}
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) {},
		}
		stats := &ps6010RunStats{}
		if err := ps6010Run(pass, stats); err != nil {
			t.Fatalf("run size %d pointer fixture: %v", size, err)
		}
		if want := size + 1; stats.pointerConstraints != want {
			t.Fatalf("size %d: got %d unique pointer constraints, want %d", size, stats.pointerConstraints, want)
		}
		if want := size + 2; stats.pointerPropagations != want {
			t.Fatalf("size %d: got %d points-to propagations, want %d", size, stats.pointerPropagations, want)
		}
		return stats.pointerConstraints + stats.pointerPropagations
	}

	var previous int
	for _, size := range []int{800, 1600, 3200} {
		current := work(size)
		if current > 2*size+4 {
			t.Fatalf("size %d: pointer-provenance work is not linear: %d", size, current)
		}
		if previous != 0 && current > 2*previous+4 {
			t.Fatalf("doubling backward pointer chain grew work superlinearly: %d -> %d", previous, current)
		}
		previous = current
	}
}

func TestPS6010PointerProvenanceGraphScaling(t *testing.T) {
	t.Parallel()

	run := func(name, source string) ps6010RunStats {
		t.Helper()
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, name+".go", source, 0)
		if err != nil {
			t.Fatalf("parse %s pointer fixture: %v", name, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check %s pointer fixture: %v", name, err)
		}
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) {},
		}
		stats := ps6010RunStats{}
		if err := ps6010Run(pass, &stats); err != nil {
			t.Fatalf("run %s pointer fixture: %v", name, err)
		}
		return stats
	}
	linearSource := func(shape string, size int) string {
		var source strings.Builder
		source.WriteString("package p\nfunc scale() {\nvar x int\n")
		for index := 0; index <= size; index++ {
			fmt.Fprintf(&source, "var p%d *int\n", index)
		}
		switch shape {
		case "sparse":
			source.WriteString("p0 = &x\n")
			for index := 1; index <= size; index++ {
				fmt.Fprintf(&source, "p%d = p%d\n", index, index-1)
			}
		case "reverse":
			for index := 0; index < size; index++ {
				fmt.Fprintf(&source, "p%d = p%d\n", index, index+1)
			}
			fmt.Fprintf(&source, "p%d = &x\n", size)
		case "cyclic":
			for index := 0; index < size; index++ {
				fmt.Fprintf(&source, "p%d = p%d\n", index, index+1)
			}
			fmt.Fprintf(&source, "p%d = p0\np0 = &x\n", size)
		}
		fmt.Fprintf(&source, "_ = p0\n_ = p%d\n}\n", size)
		return source.String()
	}
	for _, shape := range []string{"sparse", "reverse", "cyclic"} {
		var previous int
		for _, size := range []int{800, 1600, 3200} {
			stats := run(fmt.Sprintf("%s_%d", shape, size), linearSource(shape, size))
			work := stats.pointerConstraints + stats.pointerPropagations + stats.pointerEdgeVisits
			if work > 8*size+24 {
				t.Fatalf("%s size %d: pointer graph work is not linear: %+v", shape, size, stats)
			}
			if previous != 0 && work > 2*previous+24 {
				t.Fatalf("%s doubling grew pointer graph work superlinearly: %d -> %d", shape, previous, work)
			}
			previous = work
		}
	}

	var previous int
	for _, size := range []int{50, 100, 200, 400} {
		var source strings.Builder
		source.WriteString("package p\nfunc dense() {\n")
		for index := 0; index < size; index++ {
			fmt.Fprintf(&source, "var x%d int\nvar p%d *int\np%d = &x%d\n", index, index, index, index)
		}
		for left := 0; left < size; left++ {
			for right := 0; right < size; right++ {
				if left != right {
					fmt.Fprintf(&source, "p%d = p%d\n", left, right)
				}
			}
		}
		source.WriteString("_ = p0\n}\n")
		stats := run(fmt.Sprintf("dense_%d", size), source.String())
		work := stats.pointerConstraints + stats.pointerPropagations + stats.pointerEdgeVisits
		if stats.pointerConstraints != size*size {
			t.Fatalf("dense size %d: got %d unique constraints, want %d", size, stats.pointerConstraints, size*size)
		}
		if stats.pointerEdgeVisits <= stats.pointerConstraints {
			t.Fatalf("dense size %d: edge-visit counter did not expose graph scans: %+v", size, stats)
		}
		if work > 4*size*size+8*size {
			t.Fatalf("dense size %d: pointer graph work exceeds quadratic bound: %+v", size, stats)
		}
		if previous != 0 && work > 5*previous+8*size {
			t.Fatalf("dense doubling grew pointer graph work faster than quadratic: %d -> %d", previous, work)
		}
		previous = work
	}

	previous = 0
	for _, size := range []int{30, 60, 120, 240} {
		var source strings.Builder
		source.WriteString("package p\nfunc denseAcyclic() {\n")
		for index := 0; index < size; index++ {
			fmt.Fprintf(&source, "var x%d int\nvar p%d *int\np%d = &x%d\n", index, index, index, index)
		}
		for from := 0; from < size; from++ {
			for to := from + 1; to < size; to++ {
				fmt.Fprintf(&source, "p%d = p%d\n", to, from)
			}
		}
		fmt.Fprintf(&source, "_ = p0\n_ = p%d\n}\n", size-1)
		stats := run(fmt.Sprintf("dense_acyclic_%d", size), source.String())
		work := stats.pointerConstraints + stats.pointerPropagations + stats.pointerEdgeVisits
		wantConstraints := size * (size + 1) / 2
		if stats.pointerConstraints != wantConstraints {
			t.Fatalf("dense acyclic size %d: got %d constraints, want %d", size, stats.pointerConstraints, wantConstraints)
		}
		if stats.pointerEdgeVisits > 12*size*size+32*size {
			t.Fatalf("dense acyclic size %d: pointer graph edge work exceeds quadratic bound: %+v", size, stats)
		}
		if previous != 0 && work > 5*previous+32*size {
			t.Fatalf("dense acyclic doubling grew pointer graph work faster than quadratic: %d -> %d", previous, work)
		}
		previous = work
	}
}

func TestPS6010ExternalCompatibilityIndexScaling(t *testing.T) {
	t.Parallel()

	run := func(size int) ps6010RunStats {
		t.Helper()
		var source strings.Builder
		source.WriteString("package p\n")
		for index := range size {
			fmt.Fprintf(&source, "type E%d struct { E%d int }; type C%d struct { C%d string }\n", index, index, index, index)
			fmt.Fprintf(&source, "func opaque%d() []E%d { return nil }\n", index, index)
		}
		source.WriteString("func target() {\n")
		for index := range size {
			fmt.Fprintf(&source, "var p%d []C%d\n", index, index)
		}
		for index := range size {
			fmt.Fprintf(&source, "opaque%d()[0] = E%d{}\n", index, index)
		}
		for index := range size {
			fmt.Fprintf(&source, "_ = p%d\n", index)
		}
		source.WriteString("}\n")

		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "external_scale.go", source.String(), 0)
		if err != nil {
			t.Fatalf("parse size %d external fixture: %v", size, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Implicits:  make(map[ast.Node]types.Object),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check size %d external fixture: %v", size, err)
		}
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) {},
		}
		stats := ps6010RunStats{}
		if err := ps6010Run(pass, &stats); err != nil {
			t.Fatalf("run size %d external fixture: %v", size, err)
		}
		return stats
	}

	var previousTypes, previousScans int
	for _, size := range []int{50, 100, 200, 400} {
		stats := run(size)
		if stats.externalTypesClassified > 3*size+16 {
			t.Fatalf("size %d: external type classification is not linear: %+v", size, stats)
		}
		if stats.externalCandidateClassScans > 6*size+16 {
			t.Fatalf("size %d: candidate class scans are not linear: %+v", size, stats)
		}
		if previousTypes != 0 && stats.externalTypesClassified > 2*previousTypes+16 {
			t.Fatalf("doubling external types grew classifications superlinearly: %d -> %d", previousTypes, stats.externalTypesClassified)
		}
		if previousScans != 0 && stats.externalCandidateClassScans > 2*previousScans+16 {
			t.Fatalf("doubling external types grew candidate scans superlinearly: %d -> %d", previousScans, stats.externalCandidateClassScans)
		}
		previousTypes = stats.externalTypesClassified
		previousScans = stats.externalCandidateClassScans
	}
}

func TestPS6010CallableEffectWorklistScaling(t *testing.T) {
	t.Parallel()

	run := func(size int) ps6010RunStats {
		t.Helper()
		var source strings.Builder
		source.WriteString(`package p
var global []float64
func target(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		hop0(mutate, o)
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * weights[o] }
		dst[o] = acc
	}
	return dst
}
func mutate(output int) { global[0] = float64(output) }
`)
		for index := range size {
			if index+1 == size {
				fmt.Fprintf(&source, "func hop%d(callback func(int), output int) { callback(output) }\n", index)
			} else {
				fmt.Fprintf(&source, "func hop%d(callback func(int), output int) { hop%d(callback, output) }\n", index, index+1)
			}
		}

		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "callable_scale.go", source.String(), 0)
		if err != nil {
			t.Fatalf("parse size %d callable fixture: %v", size, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Implicits:  make(map[ast.Node]types.Object),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check size %d callable fixture: %v", size, err)
		}
		diagnostics := 0
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) { diagnostics++ },
		}
		stats := ps6010RunStats{}
		if err := ps6010Run(pass, &stats); err != nil {
			t.Fatalf("run size %d callable fixture: %v", size, err)
		}
		if diagnostics != 0 {
			t.Fatalf("size %d: callback mutation was not propagated through %d helpers", size, size)
		}
		return stats
	}

	previous := 0
	for _, size := range []int{50, 100, 200, 400} {
		stats := run(size)
		work := stats.callableEdges + stats.callablePropagations + stats.callableCallTargets
		if work > 16*size+32 {
			t.Fatalf("size %d: callable fixed-point work is not linear: %+v", size, stats)
		}
		if previous != 0 && work > 2*previous+32 {
			t.Fatalf("doubling callback chain grew work superlinearly: %d -> %d", previous, work)
		}
		previous = work
	}
}

func TestPS6010CallableReturnWorklistScaling(t *testing.T) {
	t.Parallel()

	run := func(size int) ps6010RunStats {
		t.Helper()
		var source strings.Builder
		source.WriteString(`package p
var global []float64
func target(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		invoke(factory0, o)
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * weights[o] }
		dst[o] = acc
	}
	return dst
}
func invoke(factory func() func(int), output int) { factory()(output) }
func mutate(output int) { global[0] = float64(output) }
`)
		for index := range size {
			if index+1 == size {
				fmt.Fprintf(&source, "func factory%d() func(int) { return mutate }\n", index)
			} else {
				fmt.Fprintf(&source, "func factory%d() func(int) { return factory%d() }\n", index, index+1)
			}
		}

		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "callable_return_scale.go", source.String(), 0)
		if err != nil {
			t.Fatalf("parse size %d callable-return fixture: %v", size, err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Scopes:     make(map[ast.Node]*types.Scope),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Implicits:  make(map[ast.Node]types.Object),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check size %d callable-return fixture: %v", size, err)
		}
		diagnostics := 0
		pass := &analysis.Pass{
			Analyzer:  PS6010.Analyzer,
			Fset:      files,
			Files:     []*ast.File{file},
			Pkg:       pkg,
			TypesInfo: info,
			Report:    func(analysis.Diagnostic) { diagnostics++ },
		}
		stats := ps6010RunStats{}
		if err := ps6010Run(pass, &stats); err != nil {
			t.Fatalf("run size %d callable-return fixture: %v", size, err)
		}
		if diagnostics != 0 {
			t.Fatalf("size %d: returned callback mutation was not propagated through %d factories", size, size)
		}
		return stats
	}

	previous := 0
	for _, size := range []int{25, 50, 100, 200} {
		stats := run(size)
		work := stats.callableEdges + stats.callablePropagations + stats.callableCallTargets
		if work > 32*size+64 {
			t.Fatalf("size %d: callable-return fixed-point work is not linear: %+v", size, stats)
		}
		if previous != 0 && work > 2*previous+64 {
			t.Fatalf("doubling factory chain grew work superlinearly: %d -> %d", previous, work)
		}
		previous = work
	}
}

func TestPS6010FreshNameVisibleScopes(t *testing.T) {
	t.Parallel()

	build := func(kind, name string) string {
		switch kind {
		case "package":
			return fmt.Sprintf("package p\nfunc f() {\nfor o := 0; o < 1; o++ { _ = o }\n_ = %s\n}\nvar %s = 42\n", name, name)
		case "import":
			return fmt.Sprintf("package p\nimport %s \"math\"\nfunc f() {\nfor o := 0; o < 1; o++ { _ = o }\n_ = %s.Pi\n}\n", name, name)
		case "outer":
			return fmt.Sprintf("package p\nfunc f() {\n%s := 42\n{\nfor o := 0; o < 1; o++ { _ = o }\n}\n_ = %s\n}\n", name, name)
		}
		return ""
	}

	for _, kind := range []string{"package", "import", "outer"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			name := "psO0"
			var source string
			for range 8 {
				source = build(kind, name)
				files := token.NewFileSet()
				file, err := parser.ParseFile(files, kind+".go", source, 0)
				if err != nil {
					t.Fatal(err)
				}
				var loop *ast.ForStmt
				ast.Inspect(file, func(node ast.Node) bool {
					if candidate, ok := node.(*ast.ForStmt); ok {
						loop = candidate
						return false
					}
					return true
				})
				physical := files.File(loop.Pos())
				next := fmt.Sprintf("psO%d", physical.Offset(loop.Pos()))
				if next == name {
					break
				}
				name = next
			}

			files := token.NewFileSet()
			file, err := parser.ParseFile(files, kind+".go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{
				Types:      make(map[ast.Expr]types.TypeAndValue),
				Defs:       make(map[*ast.Ident]types.Object),
				Uses:       make(map[*ast.Ident]types.Object),
				Scopes:     make(map[ast.Node]*types.Scope),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Implicits:  make(map[ast.Node]types.Object),
			}
			pkg, err := (&types.Config{Importer: importer.Default()}).Check("p", files, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
			facts := ps6010CollectDefinitionFacts(pass, file, nil)
			var loop *ast.ForStmt
			ast.Inspect(file, func(node ast.Node) bool {
				if candidate, ok := node.(*ast.ForStmt); ok {
					loop = candidate
					return false
				}
				return true
			})
			scope := ps6010InsertionScope(pass, facts.parents, loop)
			got, ok := ps6010FreshName(pass, scope, loop.Pos(), name)
			if !ok || got != name+"_2" {
				t.Fatalf("visible %s name not avoided: base=%q got=%q ok=%t", kind, name, got, ok)
			}
		})
	}
}

func TestPS6010FixControlFlowScopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		source  string
		wantFix bool
	}{
		{
			name: "forward goto",
			source: `package p
func f(a [4]float64, w [16]float64, out, n int, skip bool) [4]float64 {
	var dst [4]float64
	if skip { goto done }
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
		dst[o] = acc
	}
done:
	return dst
}`,
		},
		{
			name: "backward goto",
			source: `package p
func f(a [4]float64, w [16]float64, out, n int, repeat bool) [4]float64 {
	var dst [4]float64
again:
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
		dst[o] = acc
	}
	if repeat { repeat = false; goto again }
	return dst
}`,
		},
		{
			name: "cross block goto",
			source: `package p
func f(a [4]float64, w [16]float64, out, n int, skip bool) [4]float64 {
	var dst [4]float64
	if skip { goto done }
	{
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
			dst[o] = acc
		}
	}
done:
	return dst
}`,
		},
		{
			name:    "nested goto scope",
			wantFix: true,
			source: `package p
func f(a [4]float64, w [16]float64, out, n int) [4]float64 {
	var dst [4]float64
	_ = func() { goto done; done: return }
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
		dst[o] = acc
	}
	return dst
}`,
		},
		{
			name:    "enclosing labeled break continue",
			wantFix: true,
			source: `package p
func f(a [4]float64, w [16]float64, out, n int, stop bool) [4]float64 {
	var dst [4]float64
scope:
	for {
		for o := 0; o < out; o++ {
			acc := 0.0
			for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
			dst[o] = acc
		}
		if stop { break scope }
		continue scope
	}
	return dst
}`,
		},
		{
			name:    "returns around loop",
			wantFix: true,
			source: `package p
func f(a [4]float64, w [16]float64, out, n int, stop bool) [4]float64 {
	var dst [4]float64
	if stop { return dst }
	for o := 0; o < out; o++ {
		acc := 0.0
		for i := 0; i < n; i++ { acc += a[i] * w[i*out+o] }
		dst[o] = acc
	}
	if stop { return dst }
	return dst
}`,
		},
	}

	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := token.NewFileSet()
			file, err := parser.ParseFile(files, test.name+".go", test.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{
				Types:      make(map[ast.Expr]types.TypeAndValue),
				Defs:       make(map[*ast.Ident]types.Object),
				Uses:       make(map[*ast.Ident]types.Object),
				Scopes:     make(map[ast.Node]*types.Scope),
				Selections: make(map[*ast.SelectorExpr]*types.Selection),
				Implicits:  make(map[ast.Node]types.Object),
			}
			pkg, err := (&types.Config{Importer: importer.Default()}).Check("p", files, []*ast.File{file}, info)
			if err != nil {
				t.Fatalf("type-check input: %v", err)
			}
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{
				Analyzer:  PS6010.Analyzer,
				Fset:      files,
				Files:     []*ast.File{file},
				Pkg:       pkg,
				TypesInfo: info,
				Report:    func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) },
			}
			if err := ps6010Run(pass, nil); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != 1 {
				t.Fatalf("got %d diagnostics, want 1", len(diagnostics))
			}
			fixes := diagnostics[0].SuggestedFixes
			if !test.wantFix {
				if len(fixes) != 0 {
					t.Fatalf("unsafe control-flow shape received %d fixes", len(fixes))
				}
				return
			}
			if len(fixes) != 1 || len(fixes[0].TextEdits) != 1 {
				t.Fatalf("safe control-flow shape got %+v", fixes)
			}
			edit := fixes[0].TextEdits[0]
			tokenFile := files.File(edit.Pos)
			start, end := tokenFile.Offset(edit.Pos), tokenFile.Offset(edit.End)
			fixed := test.source[:start] + string(edit.NewText) + test.source[end:]
			fixedFiles := token.NewFileSet()
			fixedFile, err := parser.ParseFile(fixedFiles, test.name+"_fixed.go", fixed, 0)
			if err != nil {
				t.Fatalf("parse fixed source: %v\n%s", err, fixed)
			}
			if _, err := (&types.Config{Importer: importer.Default()}).Check("p", fixedFiles, []*ast.File{fixedFile}, nil); err != nil {
				t.Fatalf("type-check fixed source: %v\n%s", err, fixed)
			}
		})
	}
}
