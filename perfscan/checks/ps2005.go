package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2005 reports regexp.Compile/MustCompile calls inside loops. For a
// MustCompile with a constant pattern it attaches a deterministic hoist fix.
var PS2005 = register(&lint.Check{
	ID:       "PS2005",
	Category: "alloc",
	Slug:     "regexp-compile-in-loop",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a regexp.Compile/MustCompile inside a loop",
		Text: `Compiling a regular expression parses the pattern and builds the
matcher program; doing that on every loop iteration repeats work whose result
never changes for a constant pattern. Hoist the compile out of the loop (or
to package level).

The automatic fix rewrites a MustCompile with a literal pattern by binding it
to a variable immediately before the outermost enclosing loop. The rewrite is
mechanical and bit-identical: the same pattern compiles to the same matcher.
The fix is attached only when the literal pattern actually compiles — an
invalid pattern keeps the advisory report, because hoisting a MustCompile of
an invalid pattern out of a loop would move (or, for a zero-iteration loop,
introduce) the panic. Calls to Compile (two return values, error handling)
are reported but left to the reader.`,
		Before: `for _, s := range lines {
	if regexp.MustCompile("^a+$").MatchString(s) { hits++ }
}`,
		After: `psRe := regexp.MustCompile("^a+$")
for _, s := range lines {
	if psRe.MatchString(s) { hits++ }
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2005",
		Doc:  "regexp compile inside a loop",
		Run:  runPS2005,
	},
})

var regexpCompileFuncs = map[string]bool{
	"Compile":      true,
	"MustCompile":  true,
	"CompilePOSIX": true,
	// MustCompilePOSIX included for completeness.
	"MustCompilePOSIX": true,
}

func runPS2005(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "regexp", regexpCompileFuncs)
			if !ok {
				return true
			}
			loop, inLoop := astutil.InLoop(stack)
			if !inLoop {
				return true
			}
			// The pattern must be loop-invariant: a pattern that depends on
			// a variable declared inside the loop (e.g. a range variable)
			// genuinely changes per iteration and cannot be hoisted.
			if len(call.Args) >= 1 && !ps2005LoopInvariant(pass.TypesInfo, loop, call.Args[0]) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: fmt.Sprintf("regexp.%s inside a loop recompiles an invariant pattern every iteration; hoist it out of the loop", name),
			}
			if strings.HasPrefix(name, "Must") {
				if fix := hoistRegexpFix(pass.Fset, stack, call); fix != nil {
					diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
				}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2005LoopInvariant reports whether arg is invariant with respect to the
// enclosing loop: it references no variable whose declaring object lies
// within [loop.Pos(), loop.End()). Identifiers with no resolved object
// default to invariant (report — the current behavior) rather than
// suppressing.
func ps2005LoopInvariant(info *types.Info, loop ast.Node, arg ast.Expr) bool {
	invariant := true
	ast.Inspect(arg, func(n ast.Node) bool {
		if !invariant {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := info.Uses[id]
		if obj == nil {
			obj = info.Defs[id]
		}
		if obj == nil {
			return true
		}
		if _, isVar := obj.(*types.Var); !isVar {
			// Funcs, consts, type names, package names, builtins cannot
			// vary per iteration.
			return true
		}
		if loop.Pos() <= obj.Pos() && obj.Pos() < loop.End() {
			invariant = false
			return false
		}
		return true
	})
	return invariant
}

// ps2005PatternCompiles reports whether the literal pattern actually compiles
// under the same engine the call uses. A pattern that does not compile makes
// MustCompile panic; hoisting it would move (or introduce) that panic, so the
// finding must stay advisory.
func ps2005PatternCompiles(fnName string, lit *ast.BasicLit) bool {
	pat, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	if fnName == "MustCompilePOSIX" {
		_, err = regexp.CompilePOSIX(pat)
	} else {
		_, err = regexp.Compile(pat)
	}
	return err == nil
}

// hoistRegexpFix builds the two-edit hoist: insert a binding before the
// outermost enclosing loop and replace the in-loop call with the variable.
// Only literal patterns are hoisted — a pattern built from loop state is not
// invariant.
func hoistRegexpFix(fset *token.FileSet, stack []ast.Node, call *ast.CallExpr) *analysis.SuggestedFix {
	if len(call.Args) != 1 {
		return nil
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return nil
	}
	sel := call.Fun.(*ast.SelectorExpr)
	// An invalid pattern makes MustCompile panic whenever it runs. Hoisting
	// relocates that panic before the loop — and for a zero-iteration loop
	// introduces a panic the original never had. Leave those advisory.
	if !ps2005PatternCompiles(sel.Sel.Name, lit) {
		return nil
	}
	loop, ok := astutil.OutermostLoop(stack)
	if !ok {
		return nil
	}
	pos := fset.Position(call.Pos())
	name := fmt.Sprintf("psRe%d", pos.Line)
	loopPos := fset.Position(loop.Pos())
	// Assume gofmt indentation (tabs): the loop starts at column loopPos.Column,
	// i.e. loopPos.Column-1 tabs of indentation.
	indent := strings.Repeat("\t", loopPos.Column-1)
	binding := fmt.Sprintf("%s := regexp.%s(%s)\n%s", name, sel.Sel.Name, lit.Value, indent)
	return &analysis.SuggestedFix{
		Message: fmt.Sprintf("hoist regexp.%s out of the loop", sel.Sel.Name),
		TextEdits: []analysis.TextEdit{
			{Pos: loop.Pos(), End: loop.Pos(), NewText: []byte(binding)},
			{Pos: call.Pos(), End: call.End(), NewText: []byte(name)},
		},
	}
}
