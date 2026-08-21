package checks

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// builtinInScope reports whether name resolves to the predeclared (universe)
// builtin at pos — i.e. it is NOT shadowed by a local variable, a package-level
// declaration, or an import alias at that point in the source.
//
// Any AutoFix that INJECTS a call to a builtin (clear, copy, min, max, …) must
// consult this first: a user object of the same name in an enclosing scope would
// capture the injected call, silently changing behavior or breaking the build.
// PS4101 shipped without this check and rewrote loops to copy(dst, src) even
// where copy was shadowed; this is the single guard every such check now shares
// so the mistake cannot be re-made per check.
func builtinInScope(pass *analysis.Pass, pos token.Pos, name string) bool {
	if !predeclaredInScope(pass, pos, name) {
		return false
	}
	b, ok := types.Universe.Lookup(name).(*types.Builtin)
	return ok && b.Name() == name
}

// predeclaredInScope is the general universe-binding guard for fixes that
// inject a predeclared identifier. Unlike builtinInScope it also accepts
// universe type names such as string, byte, and int64. A same-named local,
// package declaration, or import alias rejects the rewrite.
func predeclaredInScope(pass *analysis.Pass, pos token.Pos, name string) bool {
	scope := pass.Pkg.Scope().Innermost(pos)
	if scope == nil {
		scope = pass.Pkg.Scope()
	}
	_, obj := scope.LookupParent(name, pos)
	return obj != nil && obj == types.Universe.Lookup(name)
}
