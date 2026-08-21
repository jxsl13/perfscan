package checks

import (
	"go/ast"
	"go/constant"
	"go/version"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5119 combines a prefix/suffix predicate and the matching branch-local trim
// into Go 1.20's CutPrefix/CutSuffix multi-result abstraction.
var PS5119 = register(&lint.Check{
	ID:       "PS5119",
	Category: "arith",
	Slug:     "guarded-prefix-suffix-trim",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "HasPrefix/HasSuffix repeats a boundary proof in a guarded TrimPrefix/TrimSuffix",
		Text: `A common prefix/suffix extraction pattern asks the standard library to
prove the same boundary twice at source level:

  if strings.HasPrefix(value, prefix) {
      rest := strings.TrimPrefix(value, prefix)
      use(rest)
  }

TrimPrefix is independently specified because it cannot inherit the surrounding
condition's proof. Since Go 1.20, strings.CutPrefix combines both operations and
returns the remainder plus the predicate result in one direct call:

  if rest, found := strings.CutPrefix(value, prefix); found {
      use(rest)
  }

This check covers Prefix and Suffix pairs in both strings and bytes. It uses a
shared typed guarded-transformer matcher: the condition must be the direct
package predicate, and the matching Trim call must be the first expression in
a single-value assignment, short declaration, or return. Both calls must use
the same ordinary package binding and the same input object. Aliases and
parentheses work; dot imports, methods, function values, user lookalikes,
existing if initializers, negated/compound conditions, delayed trims, and
cross-package or Prefix/Suffix mismatches stay silent.

The companion prefix/suffix must be stable across the two original calls. For
strings, the rule accepts the same variable object or equal compile-time
constants; a known-empty constant is excluded because there is no boundary
scan to save. For bytes, it accepts the same slice variable. Calls, selectors,
indexes, and independently constructed byte slices stay silent because
combining them would delete a potentially observable second evaluation.

The automatic fix changes only the predicate name, introduces collision-free
remainder/found variables in the if initializer, and replaces the guarded Trim
expression with that remainder. It preserves the branch and else decision,
the assignment/return scope, the original input and companion spellings, and
all other body statements. A comment inside the removed Trim expression keeps
the finding advisory. The check runs only when the file's effective language
version has the Go 1.20 Cut APIs.

Within the matched domain the rewrite is BIT-IDENTICAL. CutPrefix/CutSuffix
perform the same Has test and return the same Trim remainder. For bytes, the
result retains the same nil state, start pointer, length, capacity, and backing
array. On a miss, both forms take the else path without evaluating the
branch-local assignment or return.

Current gc can common-subexpression-eliminate the repeated comparison for the
stable operands this rule accepts, so this is primarily a directness and
optimization-robustness rule rather than a promised cycle win. The combined
API cannot lose that single-proof shape when surrounding code changes.`,
		Before: `if strings.HasPrefix(value, prefix) {
	value = strings.TrimPrefix(value, prefix)
	consume(value)
}`,
		After: `if after, found := strings.CutPrefix(value, prefix); found {
	value = after
	consume(value)
}`,
		MeasuredWin: `benchmarks/ps5119_test.go measures a matching 96 KiB
prefix on Apple M2 Pro with Go 1.26.6 (10 runs, one CPU). HasPrefix+TrimPrefix
measured a median 2,208.5 ns/op and CutPrefix 2,204 ns/op, both at 0 B/op and 0
allocations: performance parity. Objdump showed exactly one runtime.memequal
call in each benchmark, proving that this gc already folds the repeated stable
comparison. The retained benefit is directness and robustness, not a claimed
speedup on that compiler.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5119",
		Doc:  "HasPrefix/HasSuffix followed by guarded TrimPrefix/TrimSuffix instead of CutPrefix/CutSuffix",
		Run:  runPS5119,
	},
})

type ps5119Match struct {
	composition typedGuardedPackageTransformer
	pkgPath     string
	direction   string
	afterName   string
	foundName   string
}

func runPS5119(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !ps5119CutAvailable(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			match, ok := ps5119GuardedCut(pass, statement)
			if !ok {
				return true
			}
			diagnostic := analysis.Diagnostic{
				Pos: statement.Cond.Pos(), End: match.composition.transformerExpression.End(),
				Message: match.pkgPath + ".Has" + match.direction + " proves the boundary and " + match.pkgPath + ".Trim" + match.direction + " immediately repeats that proof; " + match.pkgPath + ".Cut" + match.direction + " returns the identical remainder and predicate in one direct call",
			}
			if fix, ok := ps5119SuggestedFix(file, &match); ok {
				diagnostic.SuggestedFixes = []analysis.SuggestedFix{fix}
			}
			pass.Report(diagnostic)
			return true
		})
	}
	return nil, nil
}

func ps5119GuardedCut(pass *analysis.Pass, statement *ast.IfStmt) (ps5119Match, bool) {
	for _, pkgPath := range []string{"strings", "bytes"} {
		for _, direction := range []string{"Prefix", "Suffix"} {
			composition, ok := matchTypedGuardedPackageTransformer(
				pass, statement, pkgPath, "Has"+direction, "Trim"+direction, 2,
			)
			if !ok || !ps5119SameCompanion(pass, pkgPath, composition.predicateCompanion, composition.transformerCompanion) {
				continue
			}
			afterName, foundName := ps5119FreshNames(statement)
			return ps5119Match{
				composition: composition,
				pkgPath:     pkgPath,
				direction:   direction,
				afterName:   afterName,
				foundName:   foundName,
			}, true
		}
	}
	return ps5119Match{}, false
}

func ps5119SameCompanion(pass *analysis.Pass, pkgPath string, predicate, transformer ast.Expr) bool {
	predicate = ps2110Unparen(predicate)
	transformer = ps2110Unparen(transformer)
	if pkgPath == "strings" {
		left, leftConstant := ps5119StringConstant(pass, predicate)
		right, rightConstant := ps5119StringConstant(pass, transformer)
		if leftConstant || rightConstant {
			return leftConstant && rightConstant && left != "" && left == right
		}
	}
	left, leftOK := predicate.(*ast.Ident)
	right, rightOK := transformer.(*ast.Ident)
	return leftOK && rightOK && pass.TypesInfo.ObjectOf(left) != nil &&
		pass.TypesInfo.ObjectOf(left) == pass.TypesInfo.ObjectOf(right)
}

func ps5119StringConstant(pass *analysis.Pass, expression ast.Expr) (string, bool) {
	value, ok := pass.TypesInfo.Types[expression]
	if !ok || value.Value == nil || value.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(value.Value), true
}

func ps5119FreshNames(statement *ast.IfStmt) (string, string) {
	used := make(map[string]bool)
	ast.Inspect(statement, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			used[identifier.Name] = true
		}
		return true
	})
	for suffix := 0; ; suffix++ {
		after, found := "after", "found"
		if suffix != 0 {
			number := strconv.Itoa(suffix)
			after = "after" + number
			found = "found" + number
		}
		if !used[after] && !used[found] {
			return after, found
		}
	}
}

func ps5119SuggestedFix(file *ast.File, match *ps5119Match) (analysis.SuggestedFix, bool) {
	composition := match.composition
	if ps2111CommentIn(file, composition.transformerExpression.Pos(), composition.transformerExpression.End()) {
		return analysis.SuggestedFix{}, false
	}
	return analysis.SuggestedFix{
		Message: "combine the boundary test and trim with " + match.pkgPath + ".Cut" + match.direction,
		TextEdits: []analysis.TextEdit{
			{Pos: composition.predicateExpression.Pos(), End: composition.predicateExpression.Pos(), NewText: []byte(match.afterName + ", " + match.foundName + " := ")},
			{Pos: composition.predicateSelector.Sel.Pos(), End: composition.predicateSelector.Sel.End(), NewText: []byte("Cut" + match.direction)},
			{Pos: composition.predicateExpression.End(), End: composition.predicateExpression.End(), NewText: []byte("; " + match.foundName)},
			{Pos: composition.transformerExpression.Pos(), End: composition.transformerExpression.End(), NewText: []byte(match.afterName)},
		},
	}, true
}

func ps5119CutAvailable(pass *analysis.Pass, file *ast.File) bool {
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
	return version.Compare(version.Lang(value), "go1.20") >= 0
}
