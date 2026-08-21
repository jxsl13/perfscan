package checks

import (
	"cmp"
	"go/ast"
	"go/build/constraint"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
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
SIMD calls over inferred vector-width loops. A scalar variant must itself be
architecture-specific (at most two satisfiable GOARCH values); deliberately
broad portable fallbacks therefore stay silent when a more-specific vector
sibling exists. Different signatures, overlapping partitions, unsatisfiable
constraints, scalar arithmetic without transcendental calls, unknown sibling
implementations, dot-imported math lookalikes, nested closures, and
//perfscan:architecture-symbol-gap-validated functions stay silent.

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
		After: `// Add a separately selectable arm64 vector candidate.
// Preserve exact special-value/reduction behavior and scalar tails.
// Promote only after full routed same-binary shape campaigns pass.`,
		MeasuredWin: `Owner issue #794 was validated by merged goai change #1127
(merge d3d2f68a35addbc2784c7799486a767818fef016) after two complete 15-check
matrices on archived head 6b69e5162dc7ced392bced1ddc647dee97cf3981.
The cross-sibling gap was real: adding the separately gated two-lane arm64 NEON
leaf reduced direct 32K Exp latency by 62.69-63.50% and the six-operation
geomean by 51.50-52.61% across three alternating paired count=7 M2 Pro
campaigns. Every cell reported p=0.001, 0 B/op, and 0 allocs/op. The check still
does not transfer those measurements to an unvalidated symbol or architecture.`,
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
	constraint     constraint.Expr
	constraintText string
	implicitArch   string
	implicitOS     string
	label          string
}

type ps6077Variant struct {
	source       *ps6077Source
	function     *ast.FuncDecl
	key          string
	signature    string
	scalarCalls  []string
	vectorKind   string
	vectorScore  int
	specificArch int
}

type ps6077Finding struct {
	scalar *ps6077Variant
	vector *ps6077Variant
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
	groups := make(map[string][]*ps6077Variant)
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
				key: ps6074SymbolKey(function), signature: ps6077Signature(function),
			}
			variant.scalarCalls = ps6077ScalarCalls(function, imports)
			variant.vectorKind, variant.vectorScore = ps6077VectorEvidence(function)
			variant.specificArch = len(ps6077SatisfiableArchitectures(*source))
			groups[variant.key+"|"+variant.signature] = append(groups[variant.key+"|"+variant.signature], variant)
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
				findings = append(findings, ps6077Finding{scalar: scalar, vector: best})
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
		pass.Report(analysis.Diagnostic{
			Pos: finding.scalar.function.Name.Pos(), End: finding.scalar.function.Name.End(),
			Message: finding.scalar.key + " has an architecture-specific scalar transcendental implementation (" + strings.Join(finding.scalar.scalarCalls, ", ") + ") under [" + finding.scalar.source.label + "] at " + filepath.Base(scalarPosition.Filename) + ":" + strconv.Itoa(scalarPosition.Line) + ", while the same-signature sibling is " + finding.vector.vectorKind + " under [" + finding.vector.source.label + "] at " + filepath.Base(vectorPosition.Filename) + ":" + strconv.Itoa(vectorPosition.Line) + ". This is a cross-partition scalar/vector implementation gap; add a separately selectable candidate and preserve special values, approximation/reduction order, tails, alignment, aliasing, and feature gates before routed same-binary promotion (advisory, no automatic fix)",
			Related: []analysis.RelatedInformation{
				{Pos: finding.scalar.function.Name.Pos(), End: finding.scalar.function.Name.End(), Message: "scalar transcendental sibling under " + finding.scalar.source.label},
				{Pos: finding.vector.function.Name.Pos(), End: finding.vector.function.Name.End(), Message: finding.vector.vectorKind + " sibling under " + finding.vector.source.label},
			},
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

func ps6077ScalarCalls(function *ast.FuncDecl, imports map[string]string) []string {
	if function.Body == nil {
		return nil
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
		if !ok || imports[qualifier.Name] != "math" {
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
