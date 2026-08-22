package checks

import (
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2133 reports time.LoadLocation called with a constant zone name that names a
// real IANA zone — each call re-reads and parses the timezone database.
var PS2133 = register(&lint.Check{
	ID:       "PS2133",
	Category: "alloc",
	Slug:     "loadlocation-per-call",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "time.LoadLocation with a constant zone name re-reads the tzdata on every call",
		Text: `time.LoadLocation(name) opens and parses the IANA timezone database
entry for name every time it is called — a file read (or an embedded-tzdata
lookup) plus a full zone parse. Measured for "America/New_York": ~18000 ns and
~8.6 KB / 13 allocations PER CALL, versus a single pointer load (~0.5 ns, zero
allocations) for a *time.Location cached in a package-level var.

When the zone name is a COMPILE-TIME CONSTANT the location never changes, so it
should be loaded once at package scope:

    var nyLoc, _ = time.LoadLocation("America/New_York")

and reused (t.In(nyLoc), time.Date(..., nyLoc), ...). The fast-path names "",
"UTC" and "Local" are NOT reported — LoadLocation returns the cached UTC / Local
without touching the database, so there is nothing to hoist.

DELIBERATELY advisory — no automatic fix: LoadLocation returns (*Location, error)
and the reused form must decide how to surface a load failure (a package var uses
the blank error, or an init() that logs/panics). Placing and naming that
package-level var is a restructuring for a human, the same reason PS2131/PS2132
own the analogous regexp/NewReplacer hoists rather than rewriting them.`,
		Before: `loc, _ := time.LoadLocation("America/New_York") // re-reads tzdata every call
t = t.In(loc)`,
		After: `var nyLoc, _ = time.LoadLocation("America/New_York") // package scope, loaded once

// ... then:
t = t.In(nyLoc)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2133",
		Doc:  "time.LoadLocation with a constant zone name re-reads the tzdata every call",
		Run:  runPS2133,
	},
})

// timeLoadLocationFastPaths are the zone names LoadLocation resolves without
// touching the database, so calling it with one of them is already cheap.
var timeLoadLocationFastPaths = map[string]bool{"": true, "UTC": true, "Local": true}

func runPS2133(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if _, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "time", map[string]bool{"LoadLocation": true}); !ok {
				return true
			}
			// Only a compile-time constant zone name that actually names a real
			// zone (not a fast-path) is actionable.
			v := pass.TypesInfo.Types[call.Args[0]].Value
			if v == nil || v.Kind() != constant.String {
				return true
			}
			if timeLoadLocationFastPaths[constant.StringVal(v)] {
				return true
			}
			// Only a call INSIDE a function body is per-call. A package/file
			// scope var initializer (`var loc, _ = time.LoadLocation(name)`) runs
			// exactly once at init — that IS the recommended cached form, so it
			// must not be flagged.
			inFunc := false
			for _, a := range stack {
				switch a.(type) {
				case *ast.FuncDecl, *ast.FuncLit:
					inFunc = true
				}
			}
			if !inFunc {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "time.LoadLocation with a constant zone name re-reads and parses the tzdata on every call (~18µs, ~8.6KB, 13 allocations); load it once into a package-level var (`var loc, _ = time.LoadLocation(name)`) and reuse it — advisory: the (*Location, error) return and a package var are a human-placed restructuring, so it is not applied automatically",
			})
			return true
		})
	}
	return nil, nil
}
