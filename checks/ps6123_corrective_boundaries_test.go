package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// Corrective-only witnesses of the accepted 8560f08e proof obligations.
// Original 39 and canonical16 corpora are unchanged.
func TestPS6123PermanentCorrectiveBoundaries(t *testing.T) {
	t.Parallel()
	const prelude = `package p
type Float64x2 struct{};type Mask64x2 struct{};type Int64x2 struct{}
type V=Float64x2;type M=Mask64x2;type I=Int64x2
func(V)Mul(V)V{return V{}};func(V)Div(V)V{return V{}};func(V)MulAdd(V,V)V{return V{}}
func(V)IfElse(M,V)V{return V{}};func(V)Less(V)M{return M{}}
func(M)ToInt64x2()I{return I{}};func(I)GetElem(uint8)int64{return 0}
func pureLeaf(V)V{return V{}}
`
	cases := []struct {
		name, extra, helper, caller string
		want                        int
	}{
		{"if init merges formerly distinct arms", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);small:=x;if small=e;false{return x};return small.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 0},
		{"if init keeps arms distinct", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);small:=x;if small=x;false{return x};return small.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 1},
		{"helper select cannot return past nil receive", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);result:=x.IfElse(m,e);_=<-((chan int)(nil));return result}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 0},
		{"helper select returns normally", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);result:=x.IfElse(m,e);_=1;return result}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 1},
		{"range call path breaks but alternate path falls through", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V,flag bool){for range [2]int{}{if flag{for i:=0;i<1;i++{_=erf(x)};break}}}`, 0},
		{"range call path actually repeats", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V,flag bool){for range [2]int{}{if flag{for i:=0;i<1;i++{_=erf(x)}}}}`, 1},
		{"post changes bound to stop after one call", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){limit:=2;for i:=0;i<limit;limit=0{_=erf(x)}}`, 0},
		{"unrelated post leaves repetition possible", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){limit:=2;j:=0;for i:=0;i<limit;j++{_=erf(x);if j>2{return}}}`, 1},
		{"field named Mul is not canonical pure method", `var dispatch struct{Mul func(V)V}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=dispatch.Mul(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 0},
		{"interface Mul is not canonical value intrinsic", `var dispatch interface{Mul(V)V}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=dispatch.Mul(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 0},
		{"noncanonical receiver Mul is not approved", `type Evil struct{};var dispatch Evil;var effects int;func(Evil)Mul(v V)V{effects++;return v}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=dispatch.Mul(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 0},
		{"canonical value Mul remains approved", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=x.Mul(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := prelude + tc.extra + "\n" + tc.helper + "\n" + tc.caller
			got := ps6123PermanentDiagnosticsWithPure(t, source, map[string]bool{"simd/archsimd.pureLeaf": true})
			if got != tc.want {
				t.Fatalf("diagnostics=%d want=%d\n%s", got, tc.want, source)
			}
		})
	}
}

func ps6123PermanentDiagnosticsWithPure(t *testing.T, source string, pure map[string]bool) int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "review.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
	sizes := types.SizesFor("gc", "arm64")
	if sizes == nil {
		t.Fatal("missing gc/arm64 target sizes")
	}
	pkg, err := (&types.Config{Sizes: sizes}).Check("simd/archsimd", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	pass := &analysis.Pass{Analyzer: PS6123.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(analysis.Diagnostic) { count++ }}
	if _, err := runPS6123WithPure(pass, pure); err != nil {
		t.Fatal(err)
	}
	return count
}
