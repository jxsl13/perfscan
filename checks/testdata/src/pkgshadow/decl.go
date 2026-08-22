// Package pkgshadow declares PACKAGE-LEVEL identifiers named like the
// stdlib packages the PkgFuncCall-based checks target, all in THIS file,
// while use.go calls their methods from ANOTHER file of the same package.
// The parser leaves Ident.Obj nil for cross-file references, so a purely
// syntactic qualifier match ("is it spelled fmt?") takes every call in
// use.go for a stdlib call and rewrites it — changing behavior (Sprintf
// here returns "CUSTOM") and injecting imports that collide with these
// declarations. The type-aware guard must resolve the qualifier to a
// *types.PkgName and leave use.go entirely alone: no file in this package
// may produce a diagnostic.
package pkgshadow

type myFmt struct{}

func (myFmt) Sprintf(format string, args ...any) string         { return "CUSTOM" }
func (myFmt) Sscanf(s, format string, args ...any) (int, error) { return 0, nil }

type myStrings struct{}

func (myStrings) Replace(s, old, new string, n int) string { return s }
func (myStrings) Repeat(s string, count int) string        { return s }

type mySort struct{}

func (mySort) Slice(x any, less func(i, j int) bool)       {}
func (mySort) SliceStable(x any, less func(i, j int) bool) {}

type myMath struct{}

func (myMath) Sin(x float64) float64    { return x }
func (myMath) Cos(x float64) float64    { return x }
func (myMath) Exp(x float64) float64    { return x }
func (myMath) Min(a, b float64) float64 { return a }
func (myMath) Max(a, b float64) float64 { return b }

type myRe struct{}

func (myRe) MatchString(s string) bool { return false }

type myRegexp struct{}

func (myRegexp) MustCompile(expr string) myRe { return myRe{} }

type myEndian struct{}

func (myEndian) Uint64(b []byte) uint64 { return 0 }

type myBinary struct {
	LittleEndian myEndian
}

var (
	fmt     myFmt
	strings myStrings
	sort    mySort
	math    myMath
	regexp  myRegexp
	binary  myBinary
)
