package checks

import (
	"cmp"
	"go/ast"
	"go/build/constraint"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6077 implements owner issue #794. It compares same-signature functions in
// mutually exclusive architecture/build-feature partitions and exposes an
// architecture-specific scalar transcendental implementation beside a SIMD,
// vector-width, or assembly sibling.
var PS6077 = register(&lint.Check{
	ID:       "PS6077",
	Category: "verify",
	Slug:     "architecture-symbol-scalar-vector-gap",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "same-signature architecture siblings select scalar transcendental and vector implementations",
		Text: `Per-file analysis cannot see an implementation gap hidden behind
mutually exclusive build constraints. The public Go symbol and signature can
remain identical while arm64 executes a scalar math.Exp loop and amd64 selects
assembly or a multi-lane polynomial.

This check implements owner issue #794. It parses both active and ignored Go
files belonging to the package, combines filename GOOS/GOARCH suffixes with
//go:build expressions, and checks satisfiability across the known Go operating
systems, architectures, and up to ten explicit feature tags such as
goexperiment.simd or purego. A pair is compared only when both partitions are
satisfiable and their environment sets do not overlap.

Functions are grouped by package symbol, receiver spelling, and a normalized
syntax signature that ignores ordinary parameter/result names while retaining
types, arity, variadics, results, and generic declarations. The scalar sibling
must contain a direct call through an ordinary math import to Exp/Log/Pow,
trigonometric, hyperbolic, error/gamma, or related transcendental APIs. The
other sibling must be an external assembly declaration, call a SIMD/NEON/AVX/
SSE/vector/native-named leaf, or contain a loop whose constant lane step is at
least two and whose body touches multiple indexed lanes.

The diagnostic prints the exact constraint label and file:line span of both
siblings and attaches both as related locations. It ranks assembly over named
SIMD calls over inferred vector-width loops. It also reports up to three direct
same-package consumers in the scalar partition, retaining package-function and
receiver-type identity, and up to three nearby vector leaves only when their
slice/result shape and semantic name family match the scalar symbol. Shape
identity comes from go/types for active files; ignored-file fallbacks resolve
ordinary import paths and keep unresolved or receiver-bound names distinct.
Generic receiver binders retain canonical constraint identity, and lexical
binders take precedence when they deliberately shadow a predeclared type.
Those related locations are discovery evidence: they identify where a shared
primitive may compound, but do not prove semantic interchangeability.

A scalar variant must itself be architecture-specific (at most two
satisfiable GOARCH values); deliberately broad portable fallbacks therefore
stay silent when a more-specific vector sibling exists. Different signatures,
overlapping partitions, unsatisfiable constraints, scalar arithmetic without
transcendental calls, unknown sibling implementations, dot-imported math
lookalikes, nested closures, locally shadowed calls, incompatible vector-leaf
shapes, and //perfscan:architecture-symbol-gap-validated functions stay silent.

There is NO automatic fix. A vector sibling proves an optimization family, not
semantic interchangeability. Preserve the scalar function's NaN propagation,
signed-zero ties, infinities, approximation envelope, reduction order,
alignment, tails, aliasing, and feature gates. Keep candidates separately
selectable and require instruction inspection plus same-binary alternating-
order complete-operation campaigns on every affected architecture.`,
		Before: `// exp_arm64.go: //go:build arm64 && goexperiment.simd
func ExpSumF64(x []float64) float64 {
	var sum float64
	for _, value := range x { sum += math.Exp(value) }
	return sum
}

// exp_amd64.go: //go:build amd64 && goexperiment.simd
func ExpSumF64(x []float64) float64 { return expSumAVX2(x) }`,
		After: `// Inspect related same-partition leaves and direct consumers.
// Add a separately selectable arm64 vector candidate or fused consumer.
// Preserve exact special-value/reduction behavior and scalar tails.
// Promote only after full routed same-binary shape campaigns pass.`,
		MeasuredWin: `Owner issue #794 was validated by merged goai change #1127
(merge d3d2f68a35addbc2784c7799486a767818fef016) after two complete 15-check
matrices on archived head 6b69e5162dc7ced392bced1ddc647dee97cf3981.
The cross-sibling gap was real: adding the separately gated two-lane arm64 NEON
leaf reduced direct 32K Exp latency by 62.69-63.50% and the six-operation
geomean by 51.50-52.61% across three alternating paired count=7 M2 Pro
campaigns. Every cell reported p=0.001, 0 B/op, and 0 allocs/op.

Owner issue #917 then validated shared-transcendental reuse on Apple M2 Pro:
the same two-lane logistic leaf improved production F64 sigmoid by 3.029x and
its SiLU-backward consumer by 2.087x, while a three-pass soft-cap composition
failed the production gate despite a 3.124x direct-leaf win. A fused one-pass
consumer subsequently won. Related leaves and consumers are therefore hints
for separately gated candidates, never transferred performance evidence.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6077",
		Doc:  "architecture/build-tag siblings expose scalar transcendental and SIMD implementations of the same signature",
		Run:  runPS6077,
	},
})

type ps6077Source struct {
	file           *ast.File
	filename       string
	active         bool
	constraint     constraint.Expr
	constraintText string
	implicitArch   string
	implicitOS     string
	label          string
}

type ps6077Variant struct {
	source           *ps6077Source
	function         *ast.FuncDecl
	key              string
	symbolIdentity   string
	signature        string
	scalarCalls      []string
	vectorKind       string
	vectorScore      int
	specificArch     int
	sliceResultShape ps6077Shape
	nameFamilies     map[string]bool
	allFamilies      map[string]bool
	directCalls      map[string]bool
}

type ps6077Shape struct {
	present         bool
	typed           bool
	typeParameters  int
	sliceParameters int
	results         int
	components      []string
	syntax          string
}

type ps6077Finding struct {
	scalar    *ps6077Variant
	vector    *ps6077Variant
	leaves    []*ps6077Variant
	consumers []*ps6077Variant
}

var ps6077Transcendentals = map[string]bool{
	"Acos": true, "Acosh": true, "Asin": true, "Asinh": true,
	"Atan": true, "Atan2": true, "Atanh": true, "Cbrt": true,
	"Cos": true, "Cosh": true, "Erf": true, "Erfc": true,
	"Exp": true, "Exp2": true, "Expm1": true, "Gamma": true,
	"Hypot": true, "J0": true, "J1": true, "Jn": true,
	"Lgamma": true, "Log": true, "Log10": true, "Log1p": true,
	"Log2": true, "Pow": true, "Sin": true, "Sincos": true,
	"Sinh": true, "Tan": true, "Tanh": true, "Y0": true,
	"Y1": true, "Yn": true,
}

func runPS6077(pass *analysis.Pass) (any, error) {
	sources := ps6077PackageSources(pass)
	receiverAliases := ps6077ReceiverAliases(sources)
	receiverConstraints := ps6077ReceiverTypeConstraints(sources)
	groups := make(map[string][]*ps6077Variant)
	var allVariants []*ps6077Variant
	for sourceIndex := range sources {
		source := &sources[sourceIndex]
		imports := ps6077Imports(source.file)
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || ps6077Validated(function) {
				continue
			}
			variant := &ps6077Variant{
				source: source, function: function,
				key:            ps6074SymbolKey(function),
				symbolIdentity: ps6077DeclaredSymbolIdentity(pass, source, function, receiverAliases),
				signature:      ps6077Signature(function),
			}
			variant.scalarCalls = ps6077ScalarCalls(pass, source, function, imports)
			variant.vectorKind, variant.vectorScore = ps6077VectorEvidence(function)
			variant.specificArch = len(ps6077SatisfiableArchitectures(*source))
			variant.sliceResultShape = ps6077SliceResultShape(pass, source, function, imports, receiverConstraints)
			variant.nameFamilies = ps6077SemanticFamilies(function, false)
			variant.allFamilies = ps6077SemanticFamilies(function, true)
			variant.directCalls = ps6077DirectCalls(pass, variant)
			groups[variant.key+"|"+variant.signature] = append(groups[variant.key+"|"+variant.signature], variant)
			allVariants = append(allVariants, variant)
		}
	}

	var findings []ps6077Finding
	for _, variants := range groups {
		for _, scalar := range variants {
			if len(scalar.scalarCalls) == 0 || scalar.specificArch == 0 || scalar.specificArch > 2 {
				continue
			}
			var best *ps6077Variant
			for _, vector := range variants {
				if vector.vectorScore == 0 || scalar.function == vector.function ||
					!ps6077MutuallyExclusive(*scalar.source, *vector.source) {
					continue
				}
				if best == nil || vector.vectorScore > best.vectorScore ||
					(vector.vectorScore == best.vectorScore && vector.function.Pos() < best.function.Pos()) {
					best = vector
				}
			}
			if best != nil {
				leaves, consumers := ps6077RelatedEvidence(scalar, allVariants)
				findings = append(findings, ps6077Finding{
					scalar: scalar, vector: best, leaves: leaves, consumers: consumers,
				})
			}
		}
	}
	slices.SortFunc(findings, func(left, right ps6077Finding) int {
		if byVector := cmp.Compare(right.vector.vectorScore, left.vector.vectorScore); byVector != 0 {
			return byVector
		}
		if byCalls := cmp.Compare(len(right.scalar.scalarCalls), len(left.scalar.scalarCalls)); byCalls != 0 {
			return byCalls
		}
		return cmp.Compare(left.scalar.function.Pos(), right.scalar.function.Pos())
	})
	for findingIndex := range findings {
		finding := &findings[findingIndex]
		scalarPosition := pass.Fset.Position(finding.scalar.function.Name.Pos())
		vectorPosition := pass.Fset.Position(finding.vector.function.Name.Pos())
		messageParts := make([]string, 0, 3)
		messageParts = append(messageParts, finding.scalar.key+" has an architecture-specific scalar transcendental implementation ("+strings.Join(finding.scalar.scalarCalls, ", ")+") under ["+finding.scalar.source.label+"] at "+filepath.Base(scalarPosition.Filename)+":"+strconv.Itoa(scalarPosition.Line)+", while the same-signature sibling is "+finding.vector.vectorKind+" under ["+finding.vector.source.label+"] at "+filepath.Base(vectorPosition.Filename)+":"+strconv.Itoa(vectorPosition.Line)+". This is a cross-partition scalar/vector implementation gap")
		if related := ps6077RelatedMessage(pass, finding); related != "" {
			messageParts = append(messageParts, "; "+related+". Treat related leaves and consumers as discovery evidence only: validate shared-primitive reuse or a fused consumer at the complete routed boundary")
		}
		messageParts = append(messageParts, "; add a separately selectable candidate and preserve special values, approximation/reduction order, tails, alignment, aliasing, and feature gates before routed same-binary promotion (advisory, no automatic fix)")
		related := []analysis.RelatedInformation{
			{Pos: finding.scalar.function.Name.Pos(), End: finding.scalar.function.Name.End(), Message: "scalar transcendental sibling under " + finding.scalar.source.label},
			{Pos: finding.vector.function.Name.Pos(), End: finding.vector.function.Name.End(), Message: finding.vector.vectorKind + " sibling under " + finding.vector.source.label},
		}
		for _, leaf := range finding.leaves {
			related = append(related, analysis.RelatedInformation{
				Pos: leaf.function.Name.Pos(), End: leaf.function.Name.End(),
				Message: "shape-compatible same-partition vector-leaf candidate " + leaf.key + " under " + leaf.source.label + " (semantics unproven)",
			})
		}
		for _, consumer := range finding.consumers {
			related = append(related, analysis.RelatedInformation{
				Pos: consumer.function.Name.Pos(), End: consumer.function.Name.End(),
				Message: "direct same-partition consumer " + consumer.key + " under " + consumer.source.label,
			})
		}
		pass.Report(analysis.Diagnostic{
			Pos: finding.scalar.function.Name.Pos(), End: finding.scalar.function.Name.End(),
			Message: strings.Join(messageParts, ""), Related: related,
		})
	}
	return nil, nil
}

func ps6077PackageSources(pass *analysis.Pass) []ps6077Source {
	raw := ps6074PackageSources(pass)
	result := make([]ps6077Source, 0, len(raw))
	for _, source := range raw {
		expression, text := ps6074Constraint(source.file)
		arch, operatingSystem := ps6077FilenamePartition(source.filename)
		result = append(result, ps6077Source{
			file: source.file, filename: source.filename,
			active:     source.active,
			constraint: expression, constraintText: text,
			implicitArch: arch, implicitOS: operatingSystem,
			label: ps6077ConstraintLabel(arch, operatingSystem, text),
		})
	}
	return result
}

func ps6077FilenamePartition(filename string) (string, string) {
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(filename)), ".go")
	base = strings.TrimSuffix(base, "_test")
	parts := strings.Split(base, "_")
	arch, operatingSystem := "", ""
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if slices.Contains(ps6074Architectures, last) {
			arch = last
			parts = parts[:len(parts)-1]
		}
	}
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if slices.Contains(ps6074OperatingSystems, last) {
			operatingSystem = last
		}
	}
	return arch, operatingSystem
}

func ps6077ConstraintLabel(arch, operatingSystem, expression string) string {
	var parts []string
	if operatingSystem != "" {
		parts = append(parts, "GOOS="+operatingSystem)
	}
	if arch != "" {
		parts = append(parts, "GOARCH="+arch)
	}
	if expression != "" {
		parts = append(parts, "//go:build "+expression)
	}
	if len(parts) == 0 {
		return "all builds"
	}
	return strings.Join(parts, "; ")
}

func ps6077Signature(function *ast.FuncDecl) string {
	var parts []string
	if function.Recv != nil {
		parts = append(parts, "recv="+ps6077FieldTypes(function.Recv))
	}
	if function.Type.TypeParams != nil {
		parts = append(parts, "type="+ps6077FieldTypes(function.Type.TypeParams))
	}
	parts = append(parts, "params="+ps6077FieldTypes(function.Type.Params), "results="+ps6077FieldTypes(function.Type.Results))
	return strings.Join(parts, "|")
}

func ps6077FieldTypes(fields *ast.FieldList) string {
	if fields == nil {
		return "()"
	}
	var values []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		text := exprTextRendered(field.Type)
		for range count {
			values = append(values, text)
		}
	}
	return "(" + strings.Join(values, ",") + ")"
}

func ps6077Imports(file *ast.File) map[string]string {
	result := make(map[string]string)
	for _, specification := range file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if specification.Name != nil {
			name = specification.Name.Name
		}
		if name != "." && name != "_" {
			result[name] = path
		}
	}
	return result
}

func ps6077ScalarCalls(pass *analysis.Pass, source *ps6077Source, function *ast.FuncDecl, imports map[string]string) []string {
	if function.Body == nil {
		return nil
	}
	var bindings ps6077BindingIndex
	if !source.active {
		bindings = ps6077NewBindingIndex(function)
	}
	seen := make(map[string]bool)
	var names []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
		if !ok || !ps6077Transcendentals[selector.Sel.Name] {
			return true
		}
		qualifier, ok := ps2110Unparen(selector.X).(*ast.Ident)
		if !ok || imports[qualifier.Name] != "math" || !ps6077MathSelector(pass, source, selector, qualifier, bindings) {
			return true
		}
		name := "math." + selector.Sel.Name
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return true
	})
	slices.Sort(names)
	return names
}

func ps6077MathSelector(
	pass *analysis.Pass,
	source *ps6077Source,
	selector *ast.SelectorExpr,
	qualifier *ast.Ident,
	bindings ps6077BindingIndex,
) bool {
	if source.active {
		called, ok := pass.TypesInfo.Uses[selector.Sel].(*types.Func)
		return ok && called.Pkg() != nil && called.Pkg().Path() == "math" && called.Name() == selector.Sel.Name
	}
	return !bindings.has(qualifier.Name, qualifier.Pos())
}

func ps6077VectorEvidence(function *ast.FuncDecl) (string, int) {
	if function.Body == nil {
		return "an external assembly implementation", 3
	}
	var called string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if called != "" {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ps6074CalledName(call.Fun)
		if ps6074VectorName(strings.ToLower(name)) {
			called = name
			return false
		}
		return true
	})
	if called != "" {
		return "SIMD/vector-backed via " + called, 2
	}
	if ps6077VectorWidthLoop(function.Body) {
		return "a multi-lane vector-width loop", 1
	}
	return "", 0
}

func ps6077RelatedEvidence(scalar *ps6077Variant, variants []*ps6077Variant) ([]*ps6077Variant, []*ps6077Variant) {
	shape := scalar.sliceResultShape
	families := scalar.nameFamilies
	seenLeaves := make(map[string]bool)
	seenConsumers := make(map[string]bool)
	var leaves, consumers []*ps6077Variant
	for _, candidate := range variants {
		if candidate.function == scalar.function || candidate.key == scalar.key {
			continue
		}
		leafCandidate := candidate.vectorScore > 0 && shape.present &&
			ps6077ShapesCompatible(shape, candidate.sliceResultShape) &&
			ps6077FamiliesOverlap(families, candidate.allFamilies)
		consumerCandidate := candidate.directCalls[scalar.symbolIdentity]
		if !leafCandidate && !consumerCandidate || !ps6077PartitionsOverlap(*scalar.source, *candidate.source) {
			continue
		}
		if leafCandidate {
			identity := candidate.key + "|" + candidate.signature + "|" + candidate.source.label
			if !seenLeaves[identity] {
				seenLeaves[identity] = true
				leaves = append(leaves, candidate)
			}
		}
		if consumerCandidate {
			identity := candidate.key + "|" + candidate.signature + "|" + candidate.source.label
			if !seenConsumers[identity] {
				seenConsumers[identity] = true
				consumers = append(consumers, candidate)
			}
		}
	}
	slices.SortFunc(leaves, func(left, right *ps6077Variant) int {
		if byScore := cmp.Compare(right.vectorScore, left.vectorScore); byScore != 0 {
			return byScore
		}
		return cmp.Compare(left.function.Pos(), right.function.Pos())
	})
	slices.SortFunc(consumers, func(left, right *ps6077Variant) int {
		return cmp.Compare(left.function.Pos(), right.function.Pos())
	})
	return ps6077LimitRelated(leaves), ps6077LimitRelated(consumers)
}

func ps6077LimitRelated(variants []*ps6077Variant) []*ps6077Variant {
	const maximum = 3
	if len(variants) > maximum {
		return variants[:maximum]
	}
	return variants
}

// ps6077SliceResultShape ignores scalar mode/configuration parameters so a
// mode-selected SIMD primitive can match its public composite. Requiring the
// ordered slice parameters and complete result signature keeps unrelated
// numeric helpers out of the discovery evidence.
func ps6077SliceResultShape(
	pass *analysis.Pass,
	source *ps6077Source,
	function *ast.FuncDecl,
	imports map[string]string,
	receiverConstraints ps6077ReceiverConstraintIndex,
) ps6077Shape {
	context := ps6077ShapeIdentityContext(source, function, imports, receiverConstraints)
	var parameters []string
	shape := ps6077Shape{}
	for _, field := range function.Type.Params.List {
		array, ok := ps2110Unparen(field.Type).(*ast.ArrayType)
		if !ok || array.Len != nil {
			continue
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			parameters = append(parameters, ps6077TypeIdentity(field.Type, context))
		}
	}
	if len(parameters) == 0 {
		return shape
	}
	shape.present = true
	shape.typeParameters = ps6077FieldCount(function.Type.TypeParams)
	shape.sliceParameters = len(parameters)
	shape.results = ps6077FieldCount(function.Type.Results)
	shape.syntax = "type=" + ps6077ShapeFieldTypes(function.Type.TypeParams, context) + "|slices=(" + strings.Join(parameters, ",") + ")|results=" + ps6077ShapeFieldTypes(function.Type.Results, context)
	if !source.active {
		return shape
	}
	typeBinders := ps6077TypedTypeBinders(pass, function)
	typed := true
	appendFields := func(fields *ast.FieldList, slicesOnly bool) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			if slicesOnly {
				array, ok := ps2110Unparen(field.Type).(*ast.ArrayType)
				if !ok || array.Len != nil {
					continue
				}
			}
			fieldType := pass.TypesInfo.TypeOf(field.Type)
			if fieldType == nil {
				typed = false
				continue
			}
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for range count {
				shape.components = append(shape.components, ps6077TypedTypeIdentity(fieldType, typeBinders))
			}
		}
	}
	appendFields(function.Type.TypeParams, false)
	appendFields(function.Type.Params, true)
	appendFields(function.Type.Results, false)
	shape.typed = typed
	return shape
}

func ps6077FieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		fieldCount := len(field.Names)
		if fieldCount == 0 {
			fieldCount = 1
		}
		count += fieldCount
	}
	return count
}

func ps6077ShapesCompatible(left, right ps6077Shape) bool {
	if !left.present || !right.present ||
		left.typeParameters != right.typeParameters ||
		left.sliceParameters != right.sliceParameters || left.results != right.results {
		return false
	}
	if !left.typed || !right.typed {
		return left.syntax != "" && left.syntax == right.syntax
	}
	if len(left.components) != len(right.components) {
		return false
	}
	for index := range left.components {
		if left.components[index] != right.components[index] {
			return false
		}
	}
	return true
}

func ps6077TypedTypeBinders(pass *analysis.Pass, function *ast.FuncDecl) map[*types.TypeParam]string {
	result := make(map[*types.TypeParam]string)
	object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if object == nil {
		return result
	}
	signature, _ := object.Type().(*types.Signature)
	if signature == nil {
		return result
	}
	type parameterGroup struct {
		kind       string
		parameters *types.TypeParamList
	}
	groups := []parameterGroup{
		{kind: "function", parameters: signature.TypeParams()},
		{kind: "receiver", parameters: signature.RecvTypeParams()},
	}
	for _, group := range groups {
		if group.parameters == nil {
			continue
		}
		for index := range group.parameters.Len() {
			result[group.parameters.At(index)] = ps6077TypeParameterIdentity(group.kind, index, "")
		}
	}
	decorated := make(map[*types.TypeParam]string)
	for _, group := range groups {
		if group.parameters == nil {
			continue
		}
		for index := range group.parameters.Len() {
			parameter := group.parameters.At(index)
			constraintIdentity := ps6077TypedTypeIdentity(parameter.Constraint(), result)
			decorated[parameter] = ps6077TypeParameterIdentity(group.kind, index, constraintIdentity)
		}
	}
	for parameter, identity := range decorated {
		result[parameter] = identity
	}
	return result
}

func ps6077TypeParameterIdentity(kind string, index int, constraintIdentity string) string {
	prefix := kind + "-type-parameter(" + strconv.Itoa(index)
	if constraintIdentity == "" {
		return prefix + ")"
	}
	return prefix + ",constraint=" + constraintIdentity + ")"
}

func ps6077TypedTypeIdentity(value types.Type, binders map[*types.TypeParam]string) string {
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.Basic:
		return "basic(" + typed.Name() + ")"
	case *types.Named:
		origin := typed.Origin()
		packagePath := ""
		if origin.Obj().Pkg() != nil {
			packagePath = origin.Obj().Pkg().Path()
		}
		arguments := make([]string, 0, typed.TypeArgs().Len())
		for index := range typed.TypeArgs().Len() {
			arguments = append(arguments, ps6077TypedTypeIdentity(typed.TypeArgs().At(index), binders))
		}
		return "named(" + strconv.Quote(packagePath) + "," + origin.Obj().Name() + "," + strings.Join(arguments, ",") + ")"
	case *types.TypeParam:
		if binder := binders[typed]; binder != "" {
			return binder
		}
		return "unbound-type-parameter(" + typed.Obj().Name() + "," + types.TypeString(typed.Constraint(), ps6077TypeQualifier) + ")"
	case *types.Pointer:
		return "pointer(" + ps6077TypedTypeIdentity(typed.Elem(), binders) + ")"
	case *types.Slice:
		return "slice(" + ps6077TypedTypeIdentity(typed.Elem(), binders) + ")"
	case *types.Array:
		return "array(" + strconv.FormatInt(typed.Len(), 10) + "," + ps6077TypedTypeIdentity(typed.Elem(), binders) + ")"
	case *types.Map:
		return "map(" + ps6077TypedTypeIdentity(typed.Key(), binders) + "," + ps6077TypedTypeIdentity(typed.Elem(), binders) + ")"
	case *types.Chan:
		return "chan(" + strconv.Itoa(int(typed.Dir())) + "," + ps6077TypedTypeIdentity(typed.Elem(), binders) + ")"
	case *types.Union:
		terms := make([]string, 0, typed.Len())
		for index := range typed.Len() {
			term := typed.Term(index)
			prefix := "exact"
			if term.Tilde() {
				prefix = "underlying"
			}
			terms = append(terms, prefix+"("+ps6077TypedTypeIdentity(term.Type(), binders)+")")
		}
		return "union(" + strings.Join(terms, "|") + ")"
	case *types.Interface:
		typed.Complete()
		elements := make([]string, 0, typed.NumEmbeddeds()+typed.NumExplicitMethods())
		for index := range typed.NumEmbeddeds() {
			elements = append(elements, "embedded="+ps6077TypedTypeIdentity(typed.EmbeddedType(index), binders))
		}
		for index := range typed.NumExplicitMethods() {
			method := typed.ExplicitMethod(index)
			packagePath := ""
			if method.Pkg() != nil {
				packagePath = method.Pkg().Path()
			}
			elements = append(elements, "method="+strconv.Quote(packagePath)+"."+method.Name()+":"+types.TypeString(method.Type(), ps6077TypeQualifier))
		}
		slices.Sort(elements)
		return "interface(" + strings.Join(elements, ";") + ")"
	default:
		return "typed(" + types.TypeString(value, ps6077TypeQualifier) + ")"
	}
}

func ps6077TypeQualifier(pkg *types.Package) string {
	if pkg == nil {
		return ""
	}
	return pkg.Path()
}

type ps6077TypeIdentityContext struct {
	imports     map[string]string
	binders     map[string]string
	unqualified string
}

type ps6077ReceiverConstraintIndex map[string][]string

func ps6077ShapeIdentityContext(
	source *ps6077Source,
	function *ast.FuncDecl,
	imports map[string]string,
	receiverConstraints ps6077ReceiverConstraintIndex,
) ps6077TypeIdentityContext {
	context := ps6077TypeIdentityContext{
		imports: imports, binders: make(map[string]string),
		unqualified: source.filename + "|" + function.Name.Name,
	}
	functionParameters := make([]struct {
		name       string
		index      int
		constraint ast.Expr
	}, 0)
	parameter := 0
	if function.Type.TypeParams != nil {
		for _, field := range function.Type.TypeParams.List {
			for _, name := range field.Names {
				context.binders[name.Name] = ps6077TypeParameterIdentity("function", parameter, "")
				functionParameters = append(functionParameters, struct {
					name       string
					index      int
					constraint ast.Expr
				}{name: name.Name, index: parameter, constraint: field.Type})
				parameter++
			}
		}
	}
	receiverParameters := ps6077ReceiverTypeParameters(function.Recv)
	for index, name := range receiverParameters {
		context.binders[name] = ps6077TypeParameterIdentity("receiver", index, "")
	}
	for _, typeParameter := range functionParameters {
		constraintIdentity := ps6077TypeIdentity(typeParameter.constraint, context)
		context.binders[typeParameter.name] = ps6077TypeParameterIdentity("function", typeParameter.index, constraintIdentity)
	}
	if len(receiverParameters) != 0 {
		receiverName := ps6074ReceiverName(function.Recv.List[0].Type)
		constraints := receiverConstraints[source.filename+"\x00"+receiverName]
		if len(constraints) == 0 {
			constraints = receiverConstraints[receiverName]
		}
		for index, name := range receiverParameters {
			constraintIdentity := "unresolved(" + strconv.Quote(source.filename+"|"+receiverName) + ")"
			if index < len(constraints) {
				constraintIdentity = constraints[index]
			}
			context.binders[name] = ps6077TypeParameterIdentity("receiver", index, constraintIdentity)
		}
	}
	return context
}

func ps6077ReceiverTypeConstraints(sources []ps6077Source) ps6077ReceiverConstraintIndex {
	result := make(ps6077ReceiverConstraintIndex)
	candidates := make(map[string]map[string][]string)
	for sourceIndex := range sources {
		source := &sources[sourceIndex]
		imports := ps6077Imports(source.file)
		for _, declaration := range source.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpecification, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpecification.TypeParams == nil {
					continue
				}
				constraints := ps6077SyntaxTypeParameterConstraints(
					typeSpecification.TypeParams,
					imports,
					source.filename+"|type|"+typeSpecification.Name.Name,
				)
				if len(constraints) == 0 {
					continue
				}
				result[source.filename+"\x00"+typeSpecification.Name.Name] = constraints
				if candidates[typeSpecification.Name.Name] == nil {
					candidates[typeSpecification.Name.Name] = make(map[string][]string)
				}
				key := strings.Join(constraints, "\x00")
				candidates[typeSpecification.Name.Name][key] = constraints
			}
		}
	}
	for name, identities := range candidates {
		if len(identities) != 1 {
			continue
		}
		for _, constraints := range identities {
			result[name] = constraints
		}
	}
	return result
}

func ps6077SyntaxTypeParameterConstraints(
	fields *ast.FieldList,
	imports map[string]string,
	unqualified string,
) []string {
	context := ps6077TypeIdentityContext{
		imports: imports, binders: make(map[string]string), unqualified: unqualified,
	}
	var constraints []ast.Expr
	parameter := 0
	for _, field := range fields.List {
		for _, name := range field.Names {
			context.binders[name.Name] = ps6077TypeParameterIdentity("receiver", parameter, "")
			constraints = append(constraints, field.Type)
			parameter++
		}
	}
	identities := make([]string, len(constraints))
	for index, constraint := range constraints {
		identities[index] = ps6077TypeIdentity(constraint, context)
	}
	return identities
}

func ps6077ShapeFieldTypes(fields *ast.FieldList, context ps6077TypeIdentityContext) string {
	if fields == nil {
		return "()"
	}
	var values []string
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		identity := ps6077TypeIdentity(field.Type, context)
		for range count {
			values = append(values, identity)
		}
	}
	return "(" + strings.Join(values, ",") + ")"
}

// ps6077TypeIdentity resolves file-local import aliases before comparing
// related slice/result shapes. It deliberately retains named type identity:
// two packages may expose equally spelled (or equally represented) element
// types without making their slices interchangeable.
func ps6077TypeIdentity(expression ast.Expr, context ps6077TypeIdentityContext) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		if binder := context.binders[value.Name]; binder != "" {
			return binder
		}
		if ps6077PredeclaredTypes[value.Name] {
			return "predeclared(" + value.Name + ")"
		}
		return "unresolved(" + strconv.Quote(context.unqualified) + "," + value.Name + ")"
	case *ast.SelectorExpr:
		if qualifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
			if path := context.imports[qualifier.Name]; path != "" {
				return "package(" + strconv.Quote(path) + ")." + value.Sel.Name
			}
		}
		return "selector(" + ps6077TypeIdentity(value.X, context) + ")." + value.Sel.Name
	case *ast.ArrayType:
		if value.Len == nil {
			return "slice(" + ps6077TypeIdentity(value.Elt, context) + ")"
		}
		return "array(" + ps6077TypeIdentity(value.Len, context) + "," + ps6077TypeIdentity(value.Elt, context) + ")"
	case *ast.StarExpr:
		return "pointer(" + ps6077TypeIdentity(value.X, context) + ")"
	case *ast.Ellipsis:
		return "variadic(" + ps6077TypeIdentity(value.Elt, context) + ")"
	case *ast.MapType:
		return "map(" + ps6077TypeIdentity(value.Key, context) + "," + ps6077TypeIdentity(value.Value, context) + ")"
	case *ast.ChanType:
		return "chan(" + strconv.Itoa(int(value.Dir)) + "," + ps6077TypeIdentity(value.Value, context) + ")"
	case *ast.IndexExpr:
		return "index(" + ps6077TypeIdentity(value.X, context) + "," + ps6077TypeIdentity(value.Index, context) + ")"
	case *ast.IndexListExpr:
		indices := make([]string, 0, len(value.Indices))
		for _, index := range value.Indices {
			indices = append(indices, ps6077TypeIdentity(index, context))
		}
		return "indices(" + ps6077TypeIdentity(value.X, context) + "," + strings.Join(indices, ",") + ")"
	case *ast.FuncType:
		return "func(" + ps6077ShapeFieldTypes(value.TypeParams, context) + "," + ps6077ShapeFieldTypes(value.Params, context) + "," + ps6077ShapeFieldTypes(value.Results, context) + ")"
	case *ast.InterfaceType:
		return "interface" + ps6077IdentityFields(value.Methods, context, true)
	case *ast.StructType:
		return "struct" + ps6077IdentityFields(value.Fields, context, false)
	case *ast.UnaryExpr:
		return "unary(" + value.Op.String() + "," + ps6077TypeIdentity(value.X, context) + ")"
	case *ast.BinaryExpr:
		return "binary(" + value.Op.String() + "," + ps6077TypeIdentity(value.X, context) + "," + ps6077TypeIdentity(value.Y, context) + ")"
	case *ast.BasicLit:
		return "literal(" + value.Kind.String() + "," + value.Value + ")"
	default:
		return "syntax(" + exprTextRendered(expression) + ")"
	}
}

var ps6077PredeclaredTypes = map[string]bool{
	"any": true, "bool": true, "byte": true, "comparable": true,
	"complex64": true, "complex128": true, "error": true,
	"float32": true, "float64": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true, "uintptr": true,
}

func ps6077IdentityFields(fields *ast.FieldList, context ps6077TypeIdentityContext, interfaceMethods bool) string {
	if fields == nil {
		return "()"
	}
	values := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		names := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		entry := strings.Join(names, ",") + ":" + ps6077TypeIdentity(field.Type, context)
		if !interfaceMethods && field.Tag != nil {
			values = append(values, entry+":tag="+field.Tag.Value)
			continue
		}
		values = append(values, entry)
	}
	return "(" + strings.Join(values, ";") + ")"
}

var ps6077SemanticAliases = map[string]string{
	"acos": "acos", "acosh": "acosh", "asin": "asin", "asinh": "asinh",
	"atan": "atan", "atan2": "atan", "atanh": "atanh", "cbrt": "cbrt", "cos": "cos",
	"cosh": "cosh", "erf": "erf", "erfc": "erfc", "exp": "exp",
	"exp2": "exp", "expm1": "exp", "gamma": "gamma", "gelu": "gelu", "hypot": "hypot",
	"j0": "bessel", "j1": "bessel", "jn": "bessel", "lgamma": "gamma",
	"log": "log", "log10": "log", "log1p": "log", "log2": "log", "logistic": "sigmoid", "pow": "pow",
	"sigmoid": "sigmoid", "silu": "sigmoid", "sin": "sin", "sincos": "sincos",
	"sinh": "sinh", "softplus": "softplus", "swish": "sigmoid", "tan": "tan",
	"tanh": "tanh", "y0": "bessel", "y1": "bessel", "yn": "bessel",
}

func ps6077SemanticFamilies(function *ast.FuncDecl, includeCalls bool) map[string]bool {
	result := make(map[string]bool)
	ps6077AddSemanticName(result, function.Name.Name)
	if !includeCalls || function.Body == nil {
		return result
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if ok {
			ps6077AddSemanticName(result, ps6074CalledName(call.Fun))
		}
		return true
	})
	return result
}

func ps6077AddSemanticName(families map[string]bool, name string) {
	// Go initialisms such as SiLUF64 or sharedSiLUNEON are deliberately kept
	// together by some naming styles and split as "Si"/"LU" by others. SiLU
	// and sigmoid share the same transcendental leaf, so retain that sibling
	// relationship independently of camel-case tokenization.
	if strings.Contains(strings.ToLower(name), "silu") {
		families["sigmoid"] = true
	}
	for _, word := range ps6077NameWords(name) {
		if family := ps6077SemanticAliases[word]; family != "" {
			families[family] = true
		}
	}
}

func ps6077NameWords(name string) []string {
	var words []string
	start := 0
	runes := []rune(name)
	flush := func(end int) {
		if end > start {
			words = append(words, strings.ToLower(string(runes[start:end])))
		}
		start = end
	}
	for index, current := range runes {
		if !unicode.IsLetter(current) && !unicode.IsDigit(current) {
			flush(index)
			start = index + 1
			continue
		}
		if index > start && unicode.IsUpper(current) &&
			(unicode.IsLower(runes[index-1]) || index+1 < len(runes) && unicode.IsLower(runes[index+1])) {
			flush(index)
		}
	}
	flush(len(runes))
	return words
}

func ps6077FamiliesOverlap(left, right map[string]bool) bool {
	for family := range left {
		if right[family] {
			return true
		}
	}
	return false
}

func ps6077ReceiverAliases(sources []ps6077Source) map[string]string {
	candidates := make(map[string]map[string]bool)
	for sourceIndex := range sources {
		for _, declaration := range sources[sourceIndex].file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, specification := range general.Specs {
				typeSpecification, ok := specification.(*ast.TypeSpec)
				if !ok || !typeSpecification.Assign.IsValid() {
					continue
				}
				target, ok := ps2110Unparen(typeSpecification.Type).(*ast.Ident)
				if !ok {
					continue
				}
				if candidates[typeSpecification.Name.Name] == nil {
					candidates[typeSpecification.Name.Name] = make(map[string]bool)
				}
				candidates[typeSpecification.Name.Name][target.Name] = true
			}
		}
	}
	result := make(map[string]string)
	for alias, targets := range candidates {
		if len(targets) == 1 {
			for target := range targets {
				result[alias] = target
			}
		}
	}
	return result
}

func ps6077ResolveReceiverAlias(name string, aliases map[string]string) string {
	seen := make(map[string]bool)
	for name != "" && aliases[name] != "" && !seen[name] {
		seen[name] = true
		name = aliases[name]
	}
	return name
}

func ps6077DeclaredSymbolIdentity(pass *analysis.Pass, source *ps6077Source, function *ast.FuncDecl, aliases map[string]string) string {
	if function.Recv == nil {
		return ps6077PackageFunctionIdentity(pass.Pkg.Path(), function.Name.Name)
	}
	if source.active {
		if object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func); ok {
			if identity := ps6077ObjectIdentity(object); identity != "" {
				return identity
			}
		}
	}
	receiver := ps6077ResolveReceiverAlias(ps6074ReceiverName(function.Recv.List[0].Type), aliases)
	return ps6077MethodIdentity(pass.Pkg.Path(), receiver, function.Name.Name)
}

func ps6077PackageFunctionIdentity(packagePath, name string) string {
	return "package|" + packagePath + "|" + name
}

func ps6077MethodIdentity(packagePath, receiver, name string) string {
	if receiver == "" {
		return ""
	}
	return "method|" + packagePath + "|" + receiver + "|" + name
}

func ps6077ObjectIdentity(object *types.Func) string {
	if object == nil || object.Pkg() == nil {
		return ""
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return ps6077PackageFunctionIdentity(object.Pkg().Path(), object.Name())
	}
	receiver := types.Unalias(signature.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	if named, ok := receiver.(*types.Named); ok {
		named = named.Origin()
		if named.Obj().Pkg() != nil {
			return ps6077MethodIdentity(named.Obj().Pkg().Path(), named.Obj().Name(), object.Name())
		}
	}
	return ""
}

func ps6077DirectCalls(pass *analysis.Pass, consumer *ps6077Variant) map[string]bool {
	if consumer.function.Body == nil {
		return nil
	}
	var bindings ps6077BindingIndex
	if !consumer.source.active {
		bindings = ps6077NewBindingIndex(consumer.function)
	}
	result := make(map[string]bool)
	ast.Inspect(consumer.function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if consumer.source.active {
			if identity := ps6077ActiveCallIdentity(pass, call.Fun); identity != "" {
				result[identity] = true
			}
			return true
		}
		identifier, ok := ps6077CalledIdentifier(call.Fun)
		if !ok || bindings.has(identifier.Name, identifier.Pos()) {
			return true
		}
		result[ps6077PackageFunctionIdentity(pass.Pkg.Path(), identifier.Name)] = true
		return true
	})
	return result
}

func ps6077ActiveCallIdentity(pass *analysis.Pass, expression ast.Expr) string {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		function, ok := pass.TypesInfo.Uses[value].(*types.Func)
		if !ok || function.Pkg() != pass.Pkg || function.Parent() != pass.Pkg.Scope() {
			return ""
		}
		return ps6077ObjectIdentity(function)
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		if selection == nil {
			return ""
		}
		function, _ := selection.Obj().(*types.Func)
		return ps6077ObjectIdentity(function)
	case *ast.IndexExpr:
		return ps6077ActiveCallIdentity(pass, value.X)
	case *ast.IndexListExpr:
		return ps6077ActiveCallIdentity(pass, value.X)
	default:
		return ""
	}
}

func ps6077CalledIdentifier(expression ast.Expr) (*ast.Ident, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return value, true
	case *ast.IndexExpr:
		return ps6077CalledIdentifier(value.X)
	case *ast.IndexListExpr:
		return ps6077CalledIdentifier(value.X)
	default:
		return nil, false
	}
}

func ps6077ParentMap(root ast.Node) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

type ps6077BindingRange struct {
	start token.Pos
	end   token.Pos
}

type ps6077BindingIndex map[string][]ps6077BindingRange

func ps6077NewBindingIndex(function *ast.FuncDecl) ps6077BindingIndex {
	result := make(ps6077BindingIndex)
	if function.Body == nil {
		return result
	}
	add := func(name string, start, end token.Pos) {
		if name == "" || name == "_" || start >= end {
			return
		}
		result[name] = append(result[name], ps6077BindingRange{start: start, end: end})
	}
	addFields := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, field := range fields.List {
			for _, identifier := range field.Names {
				add(identifier.Name, function.Body.Pos(), function.Body.End())
			}
		}
	}
	fieldLists := []*ast.FieldList{function.Recv, function.Type.TypeParams, function.Type.Params, function.Type.Results}
	for _, fields := range fieldLists {
		addFields(fields)
	}
	for _, name := range ps6077ReceiverTypeParameters(function.Recv) {
		add(name, function.Body.Pos(), function.Body.End())
	}
	parents := ps6077ParentMap(function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE {
				scope := ps6077BindingScope(parents, value)
				for _, expression := range value.Lhs {
					if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok && scope != nil {
						add(identifier.Name, value.End(), scope.End())
					}
				}
			}
		case *ast.RangeStmt:
			if value.Tok == token.DEFINE {
				for _, expression := range []ast.Expr{value.Key, value.Value} {
					if identifier, ok := ps2110Unparen(expression).(*ast.Ident); ok {
						add(identifier.Name, value.Body.Pos(), value.Body.End())
					}
				}
			}
		case *ast.ValueSpec:
			if scope := ps6077BindingScope(parents, value); scope != nil {
				for _, identifier := range value.Names {
					add(identifier.Name, value.End(), scope.End())
				}
			}
		case *ast.TypeSpec:
			if scope := ps6077BindingScope(parents, value); scope != nil {
				add(value.Name.Name, value.End(), scope.End())
			}
		}
		return true
	})
	for name, ranges := range result {
		slices.SortFunc(ranges, func(left, right ps6077BindingRange) int {
			if byStart := cmp.Compare(left.start, right.start); byStart != 0 {
				return byStart
			}
			return cmp.Compare(left.end, right.end)
		})
		merged := ranges[:0]
		for _, current := range ranges {
			if len(merged) == 0 || current.start > merged[len(merged)-1].end {
				merged = append(merged, current)
				continue
			}
			if current.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = current.end
			}
		}
		result[name] = merged
	}
	return result
}

func ps6077ReceiverTypeParameters(receiver *ast.FieldList) []string {
	if receiver == nil || len(receiver.List) == 0 {
		return nil
	}
	expression := ps2110Unparen(receiver.List[0].Type)
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = ps2110Unparen(pointer.X)
	}
	var arguments []ast.Expr
	switch value := expression.(type) {
	case *ast.IndexExpr:
		arguments = []ast.Expr{value.Index}
	case *ast.IndexListExpr:
		arguments = value.Indices
	}
	result := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if identifier, ok := ps2110Unparen(argument).(*ast.Ident); ok {
			result = append(result, identifier.Name)
		}
	}
	return result
}

func (index ps6077BindingIndex) has(name string, position token.Pos) bool {
	ranges := index[name]
	match := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].start > position
	})
	return match > 0 && position < ranges[match-1].end
}

func ps6077BindingScope(parents map[ast.Node]ast.Node, declaration ast.Node) ast.Node {
	if assignment, ok := declaration.(*ast.AssignStmt); ok {
		switch parent := parents[assignment].(type) {
		case *ast.IfStmt:
			if parent.Init == assignment {
				return parent
			}
		case *ast.ForStmt:
			if parent.Init == assignment {
				return parent
			}
		case *ast.SwitchStmt:
			if parent.Init == assignment {
				return parent
			}
		case *ast.TypeSwitchStmt:
			if parent.Init == assignment || parent.Assign == assignment {
				return parent
			}
		}
	}
	for node := declaration; node != nil; node = parents[node] {
		switch scope := parents[node].(type) {
		case *ast.CaseClause:
			return scope
		case *ast.CommClause:
			return scope
		case *ast.BlockStmt:
			return scope
		}
	}
	return nil
}

func ps6077RelatedMessage(pass *analysis.Pass, finding *ps6077Finding) string {
	evidence := make([]string, 0, len(finding.leaves)+len(finding.consumers))
	for _, leaf := range finding.leaves {
		position := pass.Fset.Position(leaf.function.Name.Pos())
		evidence = append(evidence, "shape-compatible "+strings.Join(ps6077SortedFamilies(ps6077SemanticFamilies(leaf.function, true)), "/")+"-family vector leaf "+leaf.key+" under ["+leaf.source.label+"] at "+filepath.Base(position.Filename)+":"+strconv.Itoa(position.Line))
	}
	for _, consumer := range finding.consumers {
		position := pass.Fset.Position(consumer.function.Name.Pos())
		evidence = append(evidence, "direct consumer "+consumer.key+" under ["+consumer.source.label+"] at "+filepath.Base(position.Filename)+":"+strconv.Itoa(position.Line))
	}
	if len(evidence) == 0 {
		return ""
	}
	return "same-partition discovery evidence: " + strings.Join(evidence, "; ")
}

func ps6077SortedFamilies(families map[string]bool) []string {
	result := make([]string, 0, len(families))
	for family := range families {
		result = append(result, family)
	}
	slices.Sort(result)
	return result
}

func ps6077VectorWidthLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		loop, ok := node.(*ast.ForStmt)
		if !ok {
			return true
		}
		step := ps6077LiteralStep(loop.Post)
		if step < 2 {
			return true
		}
		indexName := ps6077LoopIndexName(loop)
		if indexName == "" {
			return true
		}
		indexes := 0
		ast.Inspect(loop.Body, func(candidate ast.Node) bool {
			if index, ok := candidate.(*ast.IndexExpr); ok && ps6077MentionsIdentifier(index.Index, indexName) {
				indexes++
			}
			return indexes < 2
		})
		found = indexes >= 2
		return !found
	})
	return found
}

func ps6077LoopIndexName(loop *ast.ForStmt) string {
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 {
		return ""
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return ""
	}
	post, ok := loop.Post.(*ast.AssignStmt)
	if !ok || len(post.Lhs) != 1 {
		return ""
	}
	postIdentifier, ok := ps2110Unparen(post.Lhs[0]).(*ast.Ident)
	if !ok || postIdentifier.Name != identifier.Name {
		return ""
	}
	return identifier.Name
}

func ps6077MentionsIdentifier(expression ast.Expr, name string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Name == name {
			found = true
			return false
		}
		return !found
	})
	return found
}

// ps6077LiteralStep avoids type information because ignored files are parsed
// but not type-checked. Vector lane steps in architecture files are expected
// to be explicit integer literals.
func ps6077LiteralStep(statement ast.Stmt) int64 {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ADD_ASSIGN || len(assignment.Rhs) != 1 {
		return 0
	}
	literal, ok := ps2110Unparen(assignment.Rhs[0]).(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return 0
	}
	value, err := strconv.ParseInt(strings.ReplaceAll(literal.Value, "_", ""), 0, 64)
	if err != nil {
		return 0
	}
	return value
}

func ps6077MutuallyExclusive(left, right ps6077Source) bool {
	leftSatisfiable, rightSatisfiable, overlap := ps6077PartitionRelation(left, right)
	return leftSatisfiable && rightSatisfiable && !overlap
}

func ps6077PartitionsOverlap(left, right ps6077Source) bool {
	leftSatisfiable, rightSatisfiable, overlap := ps6077PartitionRelation(left, right)
	return leftSatisfiable && rightSatisfiable && overlap
}

func ps6077PartitionRelation(left, right ps6077Source) (bool, bool, bool) {
	tags := ps6077FeatureTags(left.constraint, right.constraint)
	if len(tags) > 10 {
		return false, false, false
	}
	leftSatisfiable, rightSatisfiable := false, false
	for _, arch := range ps6074Architectures {
		for _, operatingSystem := range ps6074OperatingSystems {
			for mask := 0; mask < 1<<len(tags); mask++ {
				features := ps6077FeatureMask(tags, mask)
				leftOK := ps6077Allows(left, arch, operatingSystem, features)
				rightOK := ps6077Allows(right, arch, operatingSystem, features)
				leftSatisfiable = leftSatisfiable || leftOK
				rightSatisfiable = rightSatisfiable || rightOK
				if leftOK && rightOK {
					return leftSatisfiable, rightSatisfiable, true
				}
			}
		}
	}
	return leftSatisfiable, rightSatisfiable, false
}

func ps6077SatisfiableArchitectures(source ps6077Source) map[string]bool {
	tags := ps6077FeatureTags(source.constraint)
	if len(tags) > 10 {
		return nil
	}
	arches := make(map[string]bool)
	for _, arch := range ps6074Architectures {
		for _, operatingSystem := range ps6074OperatingSystems {
			for mask := 0; mask < 1<<len(tags); mask++ {
				if ps6077Allows(source, arch, operatingSystem, ps6077FeatureMask(tags, mask)) {
					arches[arch] = true
				}
			}
		}
	}
	return arches
}

func ps6077Allows(source ps6077Source, arch, operatingSystem string, features map[string]bool) bool {
	if source.implicitArch != "" && source.implicitArch != arch ||
		source.implicitOS != "" && source.implicitOS != operatingSystem {
		return false
	}
	if source.constraint == nil {
		return true
	}
	return source.constraint.Eval(func(tag string) bool {
		switch {
		case tag == arch, tag == operatingSystem, tag == "gc":
			return true
		case tag == "unix":
			return operatingSystem != "windows" && operatingSystem != "plan9" &&
				operatingSystem != "js" && operatingSystem != "wasip1"
		case strings.HasPrefix(tag, "go1."):
			return true
		default:
			return features[tag]
		}
	})
}

func ps6077FeatureTags(expressions ...constraint.Expr) []string {
	seen := make(map[string]bool)
	for _, expression := range expressions {
		ps6077CollectTags(expression, seen)
	}
	for _, arch := range ps6074Architectures {
		delete(seen, arch)
	}
	for _, operatingSystem := range ps6074OperatingSystems {
		delete(seen, operatingSystem)
	}
	delete(seen, "unix")
	delete(seen, "gc")
	for tag := range seen {
		if strings.HasPrefix(tag, "go1.") {
			delete(seen, tag)
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	return tags
}

func ps6077CollectTags(expression constraint.Expr, seen map[string]bool) {
	switch value := expression.(type) {
	case *constraint.TagExpr:
		seen[value.Tag] = true
	case *constraint.NotExpr:
		ps6077CollectTags(value.X, seen)
	case *constraint.AndExpr:
		ps6077CollectTags(value.X, seen)
		ps6077CollectTags(value.Y, seen)
	case *constraint.OrExpr:
		ps6077CollectTags(value.X, seen)
		ps6077CollectTags(value.Y, seen)
	}
}

func ps6077FeatureMask(tags []string, mask int) map[string]bool {
	features := make(map[string]bool, len(tags))
	for index, tag := range tags {
		features[tag] = mask&(1<<index) != 0
	}
	return features
}

func ps6077Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(strings.ToLower(comment.Text), "perfscan:architecture-symbol-gap-validated") {
			return true
		}
	}
	return false
}
