package checks

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/token"
	"go/version"
	"slices"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5121 combines a Contains guard and its immediate SplitN head/tail consumer
// into one strings/bytes Cut call.
var PS5121 = register(&lint.Check{
	ID:       "PS5121",
	Category: "alloc",
	Slug:     "guarded-split-to-cut",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "Contains guards an immediate SplitN head/tail instead of using one Cut call",
		Text: `A Contains guard followed immediately by SplitN repeats the separator
scan and allocates a result slice:

  if strings.Contains(value, ":") {
      tail := strings.SplitN(value, ":", 2)[1]
      use(tail)
  }

strings.Cut returns both pieces and the predicate in one allocation-free call:

  if _, tail, found := strings.Cut(value, ":"); found {
      use(tail)
  }

This check covers index 0 and index 1, = and := assignments, and direct returns
for strings and bytes. For index 0, SplitN's count may be any compile-time
integer greater than one or negative because every such call has the same
head. Index 1 requires count 2: larger or negative counts can stop that piece
at a second separator instead of returning Cut's entire remainder. The
separator must be a proven non-empty compile-time value. strings accepts equal
constant expressions; bytes accepts equal []byte(constant-string) conversions
and constant []byte composite literals. Dynamic or empty separators stay
silent because Contains(x, empty) is always true while SplitN's empty-separator
rune explosion can return too few pieces and panic.

The shared typed guarded-indexed-producer matcher requires Contains as the
whole condition, SplitN indexing as the branch's first operation, one ordinary
package binding, and the same plain input object. Aliases and parentheses work.
Existing if initializers, compound/negated conditions, delayed consumers,
effectful assignment targets, stored piece slices, different inputs or
separators, Split/SplitAfterN, methods, function values, dot imports, and user
lookalikes stay silent.

PS5121 owns guarded index-zero assignments ahead of PS5120 so one fix pass
removes both Contains and SplitN rather than leaving a second Cut scan. The fix
introduces collision-free before/after/found variables in the if initializer
and replaces the SplitN index with the selected piece. For bytes index zero,
it emits before[:len(before):len(before)] because bytes.SplitN cap-clamps every
non-final piece while bytes.Cut does not; strings and bytes index one use the
Cut result directly. This preserves byte-slice start pointer, length, capacity,
nil state, and backing array exactly.

The original predicate operands remain byte-for-byte and keep their evaluation
order. The deleted SplitN expression contains only the same stable input plus
constant separator/count/index evaluations. Comments or last local/import uses
inside that deleted expression keep the finding advisory. A shadowed len also
keeps bytes-head findings advisory because their capacity-preserving fix
injects two builtin len calls. The rule requires the Go 1.18 Cut APIs.

Within the matched domain the rewrite is BIT-IDENTICAL, including raw invalid
UTF-8, empty heads/tails, else selection, returns, assignments, and byte-slice
alias identity. It removes one separator scan plus the SplitN result-slice
allocation.`,
		Before: `if strings.Contains(value, ":") {
	tail := strings.SplitN(value, ":", 2)[1]
	consume(tail)
}`,
		After: `if _, after, found := strings.Cut(value, ":"); found {
	tail := after
	consume(tail)
}`,
		MeasuredWin: `On Apple M2 Pro with Go 1.26.6 (10 runs, one CPU),
benchmarks/ps5121_test.go measured guarded Contains+SplitN tail extraction at a
median 33.32 ns/op, 32 B/op, and 1 allocation. The single strings.Cut form
measured 6.038 ns/op, 0 B/op, and 0 allocations: about 5.52x faster and 81.9%
less time while removing the result-slice allocation and repeated scan.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5121",
		Doc:  "Contains guard followed immediately by SplitN indexing instead of one Cut call",
		Run:  runPS5121,
	},
})

type ps5121Match struct {
	composition typedGuardedIndexedPackageProducer
	pkgPath     string
	index       int
	beforeName  string
	afterName   string
	foundName   string
}

func runPS5121(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !ps5121CutAvailable(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5121GuardedSplit(pass, statement)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.composition.resultExpression.End(),
				Message: match.pkgPath + ".Contains proves the separator before " + match.pkgPath + ".SplitN immediately scans and allocates pieces for index " + strconv.Itoa(match.index) + "; " + match.pkgPath + ".Cut returns the identical piece and predicate in one allocation-free call",
			}
			if fix, ok := ps5121SuggestedFix(pass, file, &match); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5121GuardedSplit(pass *analysis.Pass, statement *ast.IfStmt) (ps5121Match, bool) {
	for _, pkgPath := range []string{"strings", "bytes"} {
		composition, ok := matchTypedGuardedIndexedPackageProducer(pass, statement, pkgPath, "Contains", "SplitN")
		if !ok || len(composition.producer.Args) != 3 {
			continue
		}
		index, ok := ps5121Index(pass, composition.index.Index)
		if !ok {
			continue
		}
		count, ok := ps5120IntegerConstant(pass, composition.producer.Args[2])
		if !ok || count == 0 || count == 1 || index == 1 && count != 2 ||
			!ps5121SameNonemptySeparator(pass, pkgPath, composition.predicateCompanion, composition.producerCompanion) {
			continue
		}
		before, after, found := ps5121FreshNames(pass, statement)
		return ps5121Match{
			composition: composition,
			pkgPath:     pkgPath,
			index:       index,
			beforeName:  before,
			afterName:   after,
			foundName:   found,
		}, true
	}
	return ps5121Match{}, false
}

func ps5121Index(pass *analysis.Pass, expression ast.Expr) (int, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(value.Value)
	return int(integer), exact && (integer == 0 || integer == 1)
}

func ps5121SameNonemptySeparator(pass *analysis.Pass, pkgPath string, left, right ast.Expr) bool {
	if pkgPath == "strings" {
		leftValue, leftOK := ps5119StringConstant(pass, ps2110Unparen(left))
		rightValue, rightOK := ps5119StringConstant(pass, ps2110Unparen(right))
		return leftOK && rightOK && leftValue != "" && leftValue == rightValue
	}
	leftValue, leftOK := ps5121ByteConstant(pass, left)
	rightValue, rightOK := ps5121ByteConstant(pass, right)
	return leftOK && rightOK && leftValue.length != 0 && leftValue.equal(rightValue)
}

type ps5121ByteSequence struct {
	length  int64
	nonzero []ps5121ByteEntry
}

type ps5121ByteEntry struct {
	index int64
	value byte
}

func (sequence ps5121ByteSequence) equal(other ps5121ByteSequence) bool {
	if sequence.length != other.length || len(sequence.nonzero) != len(other.nonzero) {
		return false
	}
	for index, entry := range sequence.nonzero {
		if other.nonzero[index] != entry {
			return false
		}
	}
	return true
}

func ps5121ByteConstant(pass *analysis.Pass, expression ast.Expr) (ps5121ByteSequence, bool) {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.CallExpr:
		if len(value.Args) != 1 || value.Ellipsis.IsValid() || !ps2108IsByteSliceConv(pass, value.Fun) {
			return ps5121ByteSequence{}, false
		}
		constantValue, ok := pass.TypesInfo.Types[ps2110Unparen(value.Args[0])]
		if !ok || constantValue.Value == nil || constantValue.Value.Kind() != constant.String {
			return ps5121ByteSequence{}, false
		}
		text := constant.StringVal(constantValue.Value)
		sequence := ps5121ByteSequence{length: int64(len(text)), nonzero: make([]ps5121ByteEntry, 0, len(text))}
		for index := range len(text) {
			if text[index] != 0 {
				sequence.nonzero = append(sequence.nonzero, ps5121ByteEntry{index: int64(index), value: text[index]})
			}
		}
		return sequence, true
	case *ast.CompositeLit:
		if !ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(value)) {
			return ps5121ByteSequence{}, false
		}
		sequence := ps5121ByteSequence{nonzero: make([]ps5121ByteEntry, 0, len(value.Elts))}
		next := int64(0)
		for _, element := range value.Elts {
			item := element
			if keyed, ok := element.(*ast.KeyValueExpr); ok {
				index, valid := ps5120IntegerConstant(pass, keyed.Key)
				if !valid || index < 0 {
					return ps5121ByteSequence{}, false
				}
				next = index
				item = keyed.Value
			}
			integer, valid := ps5121ByteValue(pass, item)
			if !valid {
				return ps5121ByteSequence{}, false
			}
			if integer != 0 {
				sequence.nonzero = append(sequence.nonzero, ps5121ByteEntry{index: next, value: integer})
			}
			if next == 1<<63-1 {
				return ps5121ByteSequence{}, false
			}
			next++
			if next > sequence.length {
				sequence.length = next
			}
		}
		slices.SortFunc(sequence.nonzero, func(left, right ps5121ByteEntry) int {
			return cmp.Compare(left.index, right.index)
		})
		return sequence, true
	default:
		return ps5121ByteSequence{}, false
	}
}

func ps5121ByteValue(pass *analysis.Pass, expression ast.Expr) (byte, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Uint64Val(value.Value)
	return byte(integer), exact && integer <= 255
}

func ps5121FreshNames(pass *analysis.Pass, statement *ast.IfStmt) (string, string, string) {
	used := make(map[string]bool)
	ast.Inspect(statement, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			used[identifier.Name] = true
		}
		return true
	})
	for suffix := 0; ; suffix++ {
		before, after, found := "before", "after", "found"
		if suffix != 0 {
			number := strconv.Itoa(suffix)
			before, after, found = "before"+number, "after"+number, "found"+number
		}
		if !used[before] && !used[after] && !used[found] &&
			!ps5121Visible(pass, statement.Pos(), before) &&
			!ps5121Visible(pass, statement.Pos(), after) &&
			!ps5121Visible(pass, statement.Pos(), found) {
			return before, after, found
		}
	}
}

func ps5121Visible(pass *analysis.Pass, pos token.Pos, name string) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		scope = pass.Pkg.Scope()
	}
	_, object := scope.LookupParent(name, pos)
	return object != nil
}

func ps5121SuggestedFix(pass *analysis.Pass, file *ast.File, match *ps5121Match) (analysis.SuggestedFix, bool) {
	composition := match.composition
	if ps2111CommentIn(file, composition.resultExpression.Pos(), composition.resultExpression.End()) ||
		match.pkgPath == "bytes" && match.index == 0 && !builtinInScope(pass, composition.resultExpression.Pos(), "len") ||
		!deletionsKeepRequiredUses(pass, file, tokenSpan{start: composition.resultExpression.Pos(), end: composition.resultExpression.End()}) {
		return analysis.SuggestedFix{}, false
	}
	first, second := "_", "_"
	replacement := ""
	if match.index == 0 {
		first = match.beforeName
		replacement = match.beforeName
		if match.pkgPath == "bytes" {
			replacement += "[:len(" + match.beforeName + "):len(" + match.beforeName + ")]"
		}
	} else {
		second = match.afterName
		replacement = match.afterName
	}
	return analysis.SuggestedFix{
		Message: "combine Contains and SplitN with " + match.pkgPath + ".Cut",
		TextEdits: []analysis.TextEdit{
			{Pos: composition.predicateExpression.Pos(), End: composition.predicateExpression.Pos(), NewText: []byte(first + ", " + second + ", " + match.foundName + " := ")},
			{Pos: composition.predicateSelector.Sel.Pos(), End: composition.predicateSelector.Sel.End(), NewText: []byte("Cut")},
			{Pos: composition.predicateExpression.End(), End: composition.predicateExpression.End(), NewText: []byte("; " + match.foundName)},
			{Pos: composition.resultExpression.Pos(), End: composition.resultExpression.End(), NewText: []byte(replacement)},
		},
	}, true
}

func ps5121OwnsAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, parents map[ast.Node]ast.Node) bool {
	block, ok := parents[assignment].(*ast.BlockStmt)
	if !ok || len(block.List) == 0 || block.List[0] != assignment {
		return false
	}
	statement, ok := parents[block].(*ast.IfStmt)
	if !ok {
		return false
	}
	match, ok := ps5121GuardedSplit(pass, statement)
	return ok && match.composition.resultExpression == assignment.Rhs[0]
}

func ps5121CutAvailable(pass *analysis.Pass, file *ast.File) bool {
	value := ""
	if pass.TypesInfo.FileVersions != nil {
		value = pass.TypesInfo.FileVersions[file]
	}
	if value == "" && pass.Pkg != nil {
		value = pass.Pkg.GoVersion()
	}
	if value == "" || version.Lang(value) == "" {
		return true
	}
	return version.Compare(version.Lang(value), "go1.18") >= 0
}
