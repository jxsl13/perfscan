package ps2127

import "regexp"

// --- positives: fix hoists the constant-pattern MustCompile to package scope ---

func plainMatch(s string) bool {
	return regexp.MustCompile("^a+$").MatchString(s) // want `regexp\.MustCompile of a constant pattern inside function plainMatch recompiles the same matcher on every call; hoist it to a package-level var compiled once at init`
}

// docMatch carries a doc comment: the hoisted var must land ABOVE the comment,
// not between the comment and the func.
func docMatch(s string) bool {
	return regexp.MustCompilePOSIX("[0-9]+").MatchString(s) // want `regexp\.MustCompilePOSIX of a constant pattern inside function docMatch recompiles the same matcher on every call; hoist it to a package-level var compiled once at init`
}

type validator struct{}

func (validator) check(s string) bool {
	return regexp.MustCompile(`\w+`).MatchString(s) // want `regexp\.MustCompile of a constant pattern inside function check recompiles the same matcher on every call; hoist it to a package-level var compiled once at init`
}

// --- negatives: no report ---

// Already a package-level var — compiles exactly once.
var pkgRe = regexp.MustCompile("^once$")

func usesPkgRe(s string) bool { return pkgRe.MatchString(s) }

// In a loop: PS2005's domain (it hoists to the loop head).
func inLoop(lines []string) int {
	n := 0
	for _, s := range lines {
		if regexp.MustCompile("^b+$").MatchString(s) {
			n++
		}
	}
	return n
}

// init runs once at startup already.
func init() {
	_ = regexp.MustCompile("^init$")
}

// Compile returns an error — its handling cannot move to a var initializer.
func compileErr(s string) (bool, error) {
	re, err := regexp.Compile("^c+$")
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// Non-literal pattern: not a provable constant.
func dynamic(p, s string) bool {
	return regexp.MustCompile(p).MatchString(s)
}

// Shadowed qualifier: this "regexp" is a local, not the package.
func shadowed(s string) bool {
	regexp := struct {
		MustCompile func(string) *reStub
	}{MustCompile: func(string) *reStub { return &reStub{} }}
	return regexp.MustCompile("^d+$").ok
}

type reStub struct{ ok bool }

// Invalid pattern: hoisting would move a guaranteed panic from first-call to
// program init, so the fix must skip it (regression: adversarial c10).
func invalidPattern(s string) bool {
	return regexp.MustCompile("(").MatchString(s)
}

// Result bound to a variable escapes the inline-throwaway shape — it could be
// mutated via Longest() or identity-compared — so skip (regression: c21).
func boundResult(s string) bool {
	re := regexp.MustCompile("^e+$")
	return re.MatchString(s)
}

// Longest() mutates the matcher; sharing one instance would leak state across
// calls, so a Longest use is not read-only and must be skipped (regression: c21).
func longestUse() {
	regexp.MustCompile("a|ab").Longest()
}
