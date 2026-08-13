package ps2119

import . "strings"

// DOT IMPORT: the call has no selector — rng.X's call Fun is a bare
// *ast.Ident, which the check requires to be a SelectorExpr. This is a
// deliberate conservative miss (silent), never a wrong fix: renaming
// `Split` here would need the dot-imported package verified and the
// bare name rewritten, which the selector-only fix never attempts.
func dotImported(s string) {
	for _, part := range Split(s, ",") {
		process(part)
	}
}
