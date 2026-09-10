package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// This grouped matrix pins the same control-flow shape at both the helper and
// callback entry. Each case retains the ordinary owner-shaped band loop.
func TestPS6121CompositionMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, shape string
		want        int
	}{
		{"baseline", `SITE`, 1},
		{"enclosingFalseShortCircuit", `if false && len(out)>0 { SITE }`, 0},
		{"enclosingTrueShortCircuitElse", `if true || len(out)>0 {} else { SITE }`, 0},
		{"enclosingTrueShortCircuit", `if true || len(out)>0 { SITE }`, 1},
		{"enclosingFalseShortCircuitElse", `if false && len(out)>0 {} else { SITE }`, 1},
		{"prefixNotFalseShortCircuit", `if !(false && len(out)>0) { return }; SITE`, 0},
		{"prefixNotTrueShortCircuitLive", `if !(true || len(out)>0) { return }; SITE`, 1},
		{"prefixDoubleNegationTrue", `if !!(true || len(out)>0) { return }; SITE`, 0},
		{"prefixBoolEqualityTrue", `if (true || len(out)>0)==true { return }; SITE`, 0},
		{"forNotFalseShortCircuit", `for !(false && len(out)>0) {}; SITE`, 0},
		{"zeroArray", `for range [0]int{} { SITE }`, 0},
		{"oneArray", `for range [1]int{} { SITE }`, 1},
		{"zeroPointerArray", `for range new([0]int) { SITE }`, 0},
		{"onePointerArray", `for range new([1]int) { SITE }`, 1},
		{"zeroInteger", `for range 0 { SITE }`, 0},
		{"oneInteger", `for range 1 { SITE }`, 1},
		{"emptyString", `for range "" { SITE }`, 0},
		{"oneRune", `for range "ä" { SITE }`, 1},
		{"emptySlice", `for range []int{} { SITE }`, 0},
		{"oneSlice", `for range []int{1} { SITE }`, 1},
		{"emptyMap", `for range map[int]int{} { SITE }`, 0},
		{"zeroCountingFor", `for j:=0;j<0;j++ { SITE }`, 0},
		{"oneCountingFor", `for j:=0;j<1;j++ { SITE }`, 1},
		{"nonemptyArrayReturn", `for range [1]int{} { return }; SITE`, 0},
		{"nonemptyIntegerReturn", `for range 1 { return }; SITE`, 0},
		{"nonemptySliceReturn", `for range []int{1} { return }; SITE`, 0},
		{"nonemptyCountingReturn", `for j:=0;j<1;j++ { return }; SITE`, 0},
		{"emptyArrayReturnLive", `for range [0]int{} { return }; SITE`, 1},
		{"emptyIntegerReturnLive", `for range 0 { return }; SITE`, 1},
		{"zeroCountingReturnLive", `for j:=0;j<0;j++ { return }; SITE`, 1},
		{"labelBreakFromFalseFor", `outer: for true { for false { break outer } }; SITE`, 0},
		{"labelBreakFromZeroFor", `outer: for true { for j:=0;j<0;j++ { break outer } }; SITE`, 0},
		{"labelBreakFromTrueForLive", `outer: for true { for true { break outer } }; SITE`, 1},
		{"switchLabelBreakSkips", `outer: for true { switch 1 { case 1: break outer }; SITE }`, 0},
		{"switchBreakReaches", `for true { switch 1 { case 1: break }; SITE; break }`, 1},
		{"selectLabelBreakSkips", `outer: for true { select {default: break outer}; SITE }`, 0},
		{"switchBooleanTag", `switch true || len(out)>0 {case true:return;default:}; SITE`, 0},
		{"switchBooleanCase", `switch {case true || len(out)>0:return}; SITE`, 0},
		{"nilReceiveSelect", `select {case <-(chan int)(nil):}; SITE`, 0},
		{"nilSendSelect", `select {case (chan int)(nil)<-1:}; SITE`, 0},
		{"nilDefaultLive", `select {case <-(chan int)(nil):return;default:}; SITE`, 1},
		{"nilRange", `for range (chan int)(nil) {}; SITE`, 0},
	}
	for _, tc := range cases {
		for _, placement := range []string{"helper", "callback"} {
			tc, placement := tc, placement
			t.Run(placement+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				loop := `for i:=lo;i<hi;i++ {out[i]=fn(out[i],1)}`
				fanout := `parallelFor(len(out),func(lo,hi int){` + loop + `})`
				body := strings.ReplaceAll(tc.shape, "SITE", fanout)
				if placement == "callback" {
					body = `parallelFor(len(out),func(lo,hi int){` + strings.ReplaceAll(tc.shape, "SITE", loop) + `})`
				}
				source := `package repro
func parallelFor(n int, body func(int,int)){body(0,n)}
func scalar(x,y float64)float64{return x*y}
func helper(out []float64,fn func(float64,float64)float64){` + body + `}
func caller(out []float64){helper(out,scalar)}`
				if got := ps6121CompositionDiagnostics(t, source); got != tc.want {
					t.Errorf("got %d diagnostics, want %d", got, tc.want)
				}
			})
		}
	}
}

func ps6121CompositionDiagnostics(t *testing.T, source string) int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "composition.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
	pkg, err := new(types.Config).Check("repro", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	pass := &analysis.Pass{Analyzer: PS6121.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, Report: func(analysis.Diagnostic) { count++ }}
	if _, err := runPS6121WithFanout(pass, map[string]bool{"parallelFor": true}); err != nil {
		t.Fatal(err)
	}
	return count
}
