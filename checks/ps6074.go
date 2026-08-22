package checks

import (
	"cmp"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6074 implements owner issue #791. It correlates architecture-partitioned
// symbol families with ordered multi-pass callers, exposing a whole scalar row
// pass hidden beside SIMD-backed siblings on the same target architecture.
var PS6074 = register(&lint.Check{
	ID:       "PS6074",
	Category: "verify",
	Slug:     "partial-architecture-vectorization-pipeline",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an architecture pipeline mixes whole-pass scalar and SIMD-backed sibling stages",
		Text: `Inspecting one helper at a time can miss architecture skew across a
multi-pass pipeline. A softmax row may run max, exp-plus-sum, and normalize as
three shared passes, while one architecture silently selects scalar max and
normalize definitions beside a SIMD exp pass. A fully vectorized architecture
sibling proves that the symbol family has already crossed the semantic and
integration boundary, but not that copying its assembly is safe.

This check implements owner issue #791. It parses both active and ignored Go
files owned by the package and builds same-symbol families across GOARCH file
suffixes and //go:build partitions. A variant is SIMD/assembly-backed when it
is an architecture declaration implemented externally or directly calls an
AVX, SSE, NEON, SIMD, vector, native, or assembly-named helper. A scalar
variant must call an explicit scalar/fallback helper or contain a whole-slice
range or zero-to-len loop. A body that invokes a vector helper and then runs a
scalar cleanup loop remains vectorized: the cleanup is an intentional tail,
not a whole-pass fallback.

The caller correlation is deliberately strict:

  - at least three distinct partitioned symbol families are called in source
    order over the same type-resolved slice object;
  - the caller is hot by numeric-pipeline name/documentation or has a direct
    same-package call in a repeatedly evaluated loop region;
  - on one GOARCH, a whole-pass scalar stage is immediately adjacent in the
    ordered pipeline to a SIMD/assembly-backed stage; and
  - every reported scalar family has a SIMD/assembly-backed implementation on
    another architecture.

Different slices, two-stage compositions, cold neutral helpers, indirect
function values, methods on unrelated receiver types, OS-only partitions,
unknown implementations, all-scalar targets, all-vector targets, and
//perfscan:architecture-pipeline-validated callers stay silent. The diagnostic
includes repeated same-package call-site fanout so shared hot choke points can
be prioritized.

There is NO automatic fix. Assembly ABI, alignment, length cutovers, aliasing,
instruction availability, and floating-point contracts are target facts. For
max/min reductions, an ordered greater-than loop is not automatically
interchangeable with maxNum/FMAXNM or minNum/FMINNM: prove NaN skipping,
multiple NaNs, infinities, and mixed signed-zero ties bit-for-bit. Keep scalar
and vector candidates separately selectable, validate direct and routed
callers, and retain only after same-binary AB/BA campaigns across the complete
shape matrix pass a predeclared end-to-end gate.`,
		Before: `func softmaxRow(values []float32) {
	max := rowMaxF32(values)       // scalar on arm64, SIMD on amd64
	sum := expSumF32(values, max) // SIMD on arm64 and amd64
	scaleRowF32(values, 1/sum)    // scalar on arm64, SIMD on amd64
}`,
		After: `// Keep scalar and arm64-vector candidates separately selectable.
// Preserve the scalar reduction's NaN and signed-zero contract exactly.
// Retain only after full softmax routes pass same-binary AB/BA campaigns.`,
		MeasuredWin: `In three paired count-7 Apple M2 Pro campaigns from the
owner reproduction, adding 16-lane arm64 FMAXNM/FMUL/FMLA helpers to the two
missing row passes improved complete F32 softmax medians by 1.507x-2.493x
across 32x2048, 1x32000, 4x32000, 512x512, and 2048x2048. Time fell
33.64%-59.88%; 14 of 15 cells had p=.001 and the remaining cell p=.007.
Isolated 2048-element probes improved about 26x for row max, 6x for scale, and
5x for affine. Evidence: jxsl13/goai commit
edfb435ae14913135c665ba4bde1d015672371b2.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6074",
		Doc:  "ordered hot pipeline is only partially vectorized on one architecture despite vectorized symbol siblings",
		Run:  runPS6074,
	},
})

type ps6074ImplementationKind uint8

const (
	ps6074UnknownImplementation ps6074ImplementationKind = iota
	ps6074ScalarImplementation
	ps6074VectorImplementation
)

type ps6074SourceFile struct {
	file     *ast.File
	filename string
	active   bool
}

type ps6074Variant struct {
	arches map[string]bool
	kind   ps6074ImplementationKind
}

type ps6074Family struct {
	key      string
	variants []ps6074Variant
}

type ps6074StageCall struct {
	key string
}

type ps6074Target struct {
	arch       string
	scalar     []string
	vector     []string
	elsewhere  []string
	reductions []string
}

type ps6074Finding struct {
	function *ast.FuncDecl
	stages   []string
	targets  []ps6074Target
	fanout   int
}

var ps6074Architectures = []string{
	"386", "amd64", "arm", "arm64", "loong64", "mips", "mips64",
	"mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm",
}

var ps6074OperatingSystems = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios",
	"js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows",
}

func runPS6074(pass *analysis.Pass) (any, error) {
	sources := ps6074PackageSources(pass)
	families, activeKeys := ps6074SymbolFamilies(pass, sources)
	if len(families) == 0 {
		return nil, nil
	}

	declarations := make(map[*types.Func]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if ok {
				declarations[object] = function
			}
		}
	}
	loopCalls := ps6072LoopCalls(pass, declarations)
	var findings []ps6074Finding
	for object, function := range declarations {
		if ps6074Validated(function) || !ps6073Hot(function, loopCalls[object]) {
			continue
		}
		stages := ps6074OrderedStages(pass, function, activeKeys, families)
		if len(stages) < 3 {
			continue
		}
		targets := ps6074PartialTargets(stages, families)
		if len(targets) == 0 {
			continue
		}
		keys := make([]string, len(stages))
		for index, stage := range stages {
			keys[index] = stage.key
		}
		findings = append(findings, ps6074Finding{
			function: function,
			stages:   keys,
			targets:  targets,
			fanout:   loopCalls[object],
		})
	}
	slices.SortFunc(findings, func(left, right ps6074Finding) int {
		if byFanout := cmp.Compare(right.fanout, left.fanout); byFanout != 0 {
			return byFanout
		}
		return cmp.Compare(left.function.Pos(), right.function.Pos())
	})
	for _, finding := range findings {
		pass.Reportf(finding.function.Name.Pos(), "%s runs ordered passes %s over the same slice; %s; repeated same-package hot-call fanout is %d. This is a partially vectorized architecture pipeline, not a scalar tail; add separately selectable SIMD candidates and validate full routed AB/BA shape campaigns%s (advisory, no automatic fix)",
			finding.function.Name.Name,
			strings.Join(finding.stages, " -> "),
			ps6074TargetsMessage(finding.targets),
			finding.fanout,
			ps6074HazardMessage(finding.targets),
		)
	}
	return nil, nil
}

func ps6074PackageSources(pass *analysis.Pass) []ps6074SourceFile {
	sources := make([]ps6074SourceFile, 0, len(pass.Files)+len(pass.IgnoredFiles))
	seen := make(map[string]bool, len(pass.Files)+len(pass.IgnoredFiles))
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		seen[filename] = true
		sources = append(sources, ps6074SourceFile{file: file, filename: filename, active: true})
	}
	for _, filename := range pass.IgnoredFiles {
		if seen[filename] || strings.ToLower(filepath.Ext(filename)) != ".go" {
			continue
		}
		source, err := ps6053ReadFile(pass, filename)
		if err != nil {
			continue
		}
		file, err := parser.ParseFile(pass.Fset, filename, source, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil || file.Name.Name != pass.Pkg.Name() {
			continue
		}
		seen[filename] = true
		sources = append(sources, ps6074SourceFile{file: file, filename: filename})
	}
	return sources
}

func ps6074SymbolFamilies(pass *analysis.Pass, sources []ps6074SourceFile) (map[string]*ps6074Family, map[*types.Func]string) {
	families := make(map[string]*ps6074Family)
	activeKeys := make(map[*types.Func]string)
	for _, source := range sources {
		arches, partitioned := ps6074FilePartition(source.filename, source.file)
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			key := ps6074SymbolKey(function)
			if source.active {
				if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
					activeKeys[object] = key
				}
			}
			if !partitioned {
				continue
			}
			family := families[key]
			if family == nil {
				family = &ps6074Family{key: key}
				families[key] = family
			}
			family.variants = append(family.variants, ps6074Variant{
				arches: arches,
				kind:   ps6074Implementation(function),
			})
		}
	}
	for key, family := range families {
		if len(family.variants) < 2 || !ps6074FamilyHasBothKinds(family) {
			delete(families, key)
		}
	}
	return families, activeKeys
}

func ps6074FamilyHasBothKinds(family *ps6074Family) bool {
	scalar, vector := false, false
	for _, variant := range family.variants {
		scalar = scalar || variant.kind == ps6074ScalarImplementation
		vector = vector || variant.kind == ps6074VectorImplementation
	}
	return scalar && vector
}

func ps6074SymbolKey(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	receiver := ps6074ReceiverName(function.Recv.List[0].Type)
	if receiver == "" {
		return function.Name.Name
	}
	return receiver + "." + function.Name.Name
}

func ps6074ReceiverName(expression ast.Expr) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return ps6074ReceiverName(value.X)
	case *ast.IndexExpr:
		return ps6074ReceiverName(value.X)
	case *ast.IndexListExpr:
		return ps6074ReceiverName(value.X)
	default:
		return ""
	}
}

func ps6074FilePartition(filename string, file *ast.File) (map[string]bool, bool) {
	expression, expressionText := ps6074Constraint(file)
	filenameArch := ps6074FilenameArchitecture(filename)
	partitioned := filenameArch != "" || ps6074ConstraintMentionsArchitecture(expressionText)
	arches := make(map[string]bool)
	for _, arch := range ps6074Architectures {
		if filenameArch != "" && filenameArch != arch {
			continue
		}
		if expression == nil || ps6074ConstraintAllows(expression, arch) {
			arches[arch] = true
		}
	}
	return arches, partitioned
}

func ps6074Constraint(file *ast.File) (constraint.Expr, string) {
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, "//go:build ") {
				continue
			}
			expression, err := constraint.Parse(comment.Text)
			if err == nil {
				return expression, strings.TrimSpace(strings.TrimPrefix(comment.Text, "//go:build"))
			}
		}
	}
	return nil, ""
}

func ps6074FilenameArchitecture(filename string) string {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(filename)), ".go")
	base = strings.TrimSuffix(base, "_test")
	for _, arch := range ps6074Architectures {
		if strings.HasSuffix(base, "_"+arch) {
			return arch
		}
	}
	return ""
}

func ps6074ConstraintMentionsArchitecture(expression string) bool {
	for _, arch := range ps6074Architectures {
		if ps6068ContainsWord(expression, arch) {
			return true
		}
	}
	return false
}

func ps6074ConstraintAllows(expression constraint.Expr, arch string) bool {
	for _, operatingSystem := range ps6074OperatingSystems {
		if expression.Eval(func(tag string) bool {
			switch {
			case tag == arch, tag == operatingSystem, tag == "gc":
				return true
			case tag == "unix":
				return operatingSystem != "windows" && operatingSystem != "plan9" && operatingSystem != "js" && operatingSystem != "wasip1"
			case strings.HasPrefix(tag, "go1."):
				return true
			default:
				return false
			}
		}) {
			return true
		}
	}
	return false
}

func ps6074Implementation(function *ast.FuncDecl) ps6074ImplementationKind {
	if function.Body == nil {
		return ps6074VectorImplementation
	}
	vector, scalar := false, false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := strings.ToLower(ps6074CalledName(call.Fun))
		vector = vector || ps6074VectorName(name)
		scalar = scalar || ps6074ScalarName(name)
		return true
	})
	if vector {
		return ps6074VectorImplementation
	}
	if scalar || ps6074WholeSliceLoop(function) {
		return ps6074ScalarImplementation
	}
	return ps6074UnknownImplementation
}

func ps6074CalledName(expression ast.Expr) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.IndexExpr:
		return ps6074CalledName(value.X)
	case *ast.IndexListExpr:
		return ps6074CalledName(value.X)
	default:
		return ""
	}
}

func ps6074VectorName(name string) bool {
	return ps6007ContainsAny(name,
		"avx", "sse", "neon", "simd", "vector", "assembly", "native",
		"fmaxnm", "fminnm", "fmla", "x4", "x8", "x16",
	) || strings.HasPrefix(name, "asm") || strings.HasSuffix(name, "asm") || strings.Contains(name, "_asm")
}

func ps6074ScalarName(name string) bool {
	return ps6007ContainsAny(name, "scalar", "fallback", "purego", "generic")
}

func ps6074WholeSliceLoop(function *ast.FuncDecl) bool {
	parameters := ps6074SliceParameters(function)
	if len(parameters) == 0 || strings.Contains(strings.ToLower(function.Name.Name), "tail") {
		return false
	}
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch loop := node.(type) {
		case *ast.RangeStmt:
			identifier, ok := ps2110Unparen(loop.X).(*ast.Ident)
			found = ok && parameters[identifier.Name]
		case *ast.ForStmt:
			found = ps6074ZeroToLenLoop(loop, parameters)
		}
		return !found
	})
	return found
}

func ps6074SliceParameters(function *ast.FuncDecl) map[string]bool {
	result := make(map[string]bool)
	if function.Type.Params == nil {
		return result
	}
	for _, field := range function.Type.Params.List {
		array, ok := ps2110Unparen(field.Type).(*ast.ArrayType)
		if !ok || array.Len != nil {
			continue
		}
		for _, name := range field.Names {
			result[name.Name] = true
		}
	}
	return result
}

func ps6074ZeroToLenLoop(loop *ast.ForStmt, parameters map[string]bool) bool {
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 ||
		!ps6074Zero(initializer.Rhs[0]) {
		return false
	}
	index, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return false
	}
	postIndex, ok := ps2110Unparen(post.X).(*ast.Ident)
	if !ok || postIndex.Name != index.Name {
		return false
	}
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS {
		return false
	}
	conditionIndex, ok := ps2110Unparen(condition.X).(*ast.Ident)
	if !ok || conditionIndex.Name != index.Name {
		return false
	}
	call, ok := ps2110Unparen(condition.Y).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || ps6074CalledName(call.Fun) != "len" {
		return false
	}
	parameter, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	return ok && parameters[parameter.Name]
}

func ps6074Zero(expression ast.Expr) bool {
	literal, ok := ps2110Unparen(expression).(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == "0"
}

func ps6074OrderedStages(pass *analysis.Pass, function *ast.FuncDecl, activeKeys map[*types.Func]string, families map[string]*ps6074Family) []ps6074StageCall {
	bySlice := make(map[types.Object][]ps6074StageCall)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, _, ok := typedCallee(pass, call.Fun)
		key := activeKeys[callee]
		if !ok || families[key] == nil {
			return true
		}
		seen := make(map[types.Object]bool, len(call.Args))
		for _, argument := range call.Args {
			object := ps6074SliceObject(pass, argument)
			if object == nil || seen[object] {
				continue
			}
			seen[object] = true
			bySlice[object] = append(bySlice[object], ps6074StageCall{key: key})
		}
		return true
	})
	var best []ps6074StageCall
	for _, calls := range bySlice {
		seen := make(map[string]bool, len(calls))
		distinct := make([]ps6074StageCall, 0, len(calls))
		for _, call := range calls {
			if seen[call.key] {
				continue
			}
			seen[call.key] = true
			distinct = append(distinct, call)
		}
		if len(distinct) > len(best) {
			best = distinct
		}
	}
	return best
}

func ps6074SliceObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == nil {
			return nil
		}
		if _, ok := types.Unalias(object.Type()).Underlying().(*types.Slice); ok {
			return object
		}
	case *ast.SliceExpr:
		return ps6074SliceObject(pass, value.X)
	}
	return nil
}

func ps6074PartialTargets(stages []ps6074StageCall, families map[string]*ps6074Family) []ps6074Target {
	var targets []ps6074Target
	for _, arch := range ps6074Architectures {
		kinds := make([]ps6074ImplementationKind, len(stages))
		for index, stage := range stages {
			kinds[index] = ps6074KindOn(families[stage.key], arch)
		}
		target := ps6074Target{arch: arch}
		seenScalar := make(map[string]bool)
		seenVector := make(map[string]bool)
		seenReduction := make(map[string]bool)
		var commonElsewhere map[string]bool
		for index, kind := range kinds {
			if kind != ps6074ScalarImplementation || !ps6074AdjacentVector(kinds, index) {
				continue
			}
			key := stages[index].key
			elsewhere := ps6074VectorElsewhere(families[key], arch)
			if len(elsewhere) == 0 {
				continue
			}
			if commonElsewhere == nil {
				commonElsewhere = elsewhere
			} else {
				intersection := make(map[string]bool)
				for candidate := range commonElsewhere {
					if elsewhere[candidate] {
						intersection[candidate] = true
					}
				}
				if len(intersection) == 0 {
					continue
				}
				commonElsewhere = intersection
			}
			if !seenScalar[key] {
				target.scalar = append(target.scalar, key)
				seenScalar[key] = true
			}
			if ps6074ReductionFamily(key) && !seenReduction[key] {
				target.reductions = append(target.reductions, key)
				seenReduction[key] = true
			}
			for neighbor := max(0, index-1); neighbor <= min(len(stages)-1, index+1); neighbor++ {
				neighborKey := stages[neighbor].key
				if kinds[neighbor] == ps6074VectorImplementation && !seenVector[neighborKey] {
					target.vector = append(target.vector, neighborKey)
					seenVector[neighborKey] = true
				}
			}
		}
		for elsewhere := range commonElsewhere {
			target.elsewhere = append(target.elsewhere, elsewhere)
		}
		slices.Sort(target.elsewhere)
		if len(target.scalar) != 0 && len(target.vector) != 0 {
			targets = append(targets, target)
		}
	}
	return targets
}

func ps6074KindOn(family *ps6074Family, arch string) ps6074ImplementationKind {
	scalar, vector := false, false
	for _, variant := range family.variants {
		if !variant.arches[arch] {
			continue
		}
		scalar = scalar || variant.kind == ps6074ScalarImplementation
		vector = vector || variant.kind == ps6074VectorImplementation
	}
	if scalar == vector {
		return ps6074UnknownImplementation
	}
	if scalar {
		return ps6074ScalarImplementation
	}
	return ps6074VectorImplementation
}

func ps6074AdjacentVector(kinds []ps6074ImplementationKind, index int) bool {
	return index > 0 && kinds[index-1] == ps6074VectorImplementation ||
		index+1 < len(kinds) && kinds[index+1] == ps6074VectorImplementation
}

func ps6074VectorElsewhere(family *ps6074Family, target string) map[string]bool {
	result := make(map[string]bool)
	for _, arch := range ps6074Architectures {
		if arch != target && ps6074KindOn(family, arch) == ps6074VectorImplementation {
			result[arch] = true
		}
	}
	return result
}

func ps6074ReductionFamily(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "max") || strings.Contains(key, "min")
}

func ps6074TargetsMessage(targets []ps6074Target) string {
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		parts = append(parts, target.arch+" selects whole-pass scalar "+strings.Join(target.scalar, ", ")+
			" beside SIMD/assembly-backed "+strings.Join(target.vector, ", ")+
			" while the scalar families are vectorized on "+strings.Join(target.elsewhere, ", "))
	}
	return strings.Join(parts, "; ")
}

func ps6074HazardMessage(targets []ps6074Target) string {
	seen := make(map[string]bool)
	var reductions []string
	for _, target := range targets {
		for _, reduction := range target.reductions {
			if !seen[reduction] {
				seen[reduction] = true
				reductions = append(reductions, reduction)
			}
		}
	}
	if len(reductions) == 0 {
		return ""
	}
	slices.Sort(reductions)
	return "; before replacing ordered comparisons in " + strings.Join(reductions, ", ") +
		" with maxNum/minNum or FMAXNM/FMINNM, prove NaN skipping and mixed signed-zero ties bit-for-bit"
}

func ps6074Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(comment.Text, "perfscan:architecture-pipeline-validated") {
			return true
		}
	}
	return false
}
