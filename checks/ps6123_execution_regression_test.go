package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"testing"
)

// Reviewer-owned bounded composition batch. Canonical SDK signatures and exact
// target Sizes are part of the fixture, not assumptions made by production.
// This table is frozen before testing; each snippet must independently typecheck.
func TestPS6123ExecutionRegression(t *testing.T) {
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
		name, extra, helper, caller, arch string
		want                              int
	}{
		{"assignment post one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<1;i=1{_=erf(x)}}`, "arm64", 0},
		{"assignment post genuinely repeats", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<1;i=0{_=erf(x)}}`, "arm64", 1},
		{"mutable bound cuts off after first", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){limit:=2;for i:=0;i<limit;i++{_=erf(x);limit=1}}`, "arm64", 0},
		{"stable bound may repeat", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){limit:=2;for i:=0;i<limit;i++{_=erf(x)}}`, "arm64", 1},
		{"uint64 wrap one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uint64(18446744073709551615);i>0;i++{_=erf(x)}}`, "arm64", 0},
		{"uint64 two iterations near wrap", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uint64(18446744073709551614);i>0;i++{_=erf(x)}}`, "arm64", 1},
		{"int target32 wrap one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=int(2147483647);i>0;i++{_=erf(x)}}`, "386", 0},
		{"int target64 same source may repeat", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=int(2147483647);i>0;i++{_=erf(x)}}`, "arm64", 1},
		{"uint target32 wrap one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uint(4294967295);i>0;i++{_=erf(x)}}`, "386", 0},
		{"uint target64 same source may repeat", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uint(4294967295);i>0;i++{_=erf(x)}}`, "arm64", 1},
		{"uintptr target32 wrap one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uintptr(4294967295);i>0;i++{_=erf(x)}}`, "386", 0},
		{"uintptr target64 same source may repeat", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=uintptr(4294967295);i>0;i++{_=erf(x)}}`, "arm64", 1},
		{"outer range terminated after one inner iteration", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for range [2]int{}{for i:=0;i<1;i++{_=erf(x)};break}}`, "arm64", 0},
		{"outer range actually repeats one trip inner", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for range [2]int{}{for i:=0;i<1;i++{_=erf(x)}}}`, "arm64", 1},
		{"caller IIFE inner loop repeats", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){func(){for i:=0;i<2;i++{_=erf(x)}}()}`, "arm64", 1},
		{"caller IIFE inner loop one trip", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){func(){for i:=0;i<1;i++{_=erf(x)}}()}`, "arm64", 0},
		{"nested synchronous IIFE repeated outer", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{func(){func(){_=erf(x)}()}()}}`, "arm64", 1},
		{"nested IIFE dead target", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{func(){func(){return;_=erf(x)}()}()}}`, "arm64", 0},
		{"nested IIFE parent blocking suffix", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{func(){func(){_=erf(x)}();<-((chan int)(nil))}()}}`, "arm64", 0},
		{"zero outer loop IIFE repeated inner", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for range [0]int{}{func(){for i:=0;i<2;i++{_=erf(x)}}()}}`, "arm64", 0},
		{"helper unconditional return makes candidate dead", ``, `func erf(x V)V{return x;m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper normal live candidate", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"helper true return prefix", ``, `func erf(x V)V{if true{return x};m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper false return prefix live", ``, `func erf(x V)V{if false{return x};m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"helper blocking condition prefix", ``, `func erf(x V)V{if func()bool{select{}}(){return x};m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper blocking if init prefix", ``, `func erf(x V)V{if _ = <-((chan int)(nil));false{return x};m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper false ignored init live", ``, `func erf(x V)V{if _ = 1;false{return x};m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"helper direct nil receive prefix", ``, `func erf(x V)V{_ = <-((chan int)(nil));m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper live receive-free prefix", ``, `func erf(x V)V{_ = 1;m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"helper candidate short circuit dead", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);_ = false && (x.IfElse(m,e)==x);return x}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"helper candidate short circuit live", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);_ = true && (x.IfElse(m,e)==x);return x}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"unconfigured side effecting vector method", `var effects int;func(V)Opaque(v V)V{effects++;return v}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=x.Opaque(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"canonical value method control", ``, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=x.Mul(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"dynamic selector function field", `var dispatch struct{fn func(V)V}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=dispatch.fn(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"interface selector method", `var dispatch interface{Opaque(V)V}`, `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=dispatch.Opaque(e);return x.IfElse(m,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"global result assignment is not local pure version", `var globalResult V`, `func erf(x V)V{m:=x.Less(x);globalResult=pureLeaf(x);return x.IfElse(m,globalResult)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"local result assignment remains pure", ``, `func erf(x V)V{m:=x.Less(x);result:=x;result=pureLeaf(x);return x.IfElse(m,result)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 1},
		{"guard cannot use stale global mask through opaque mutation", `var globalMask M;func mutate()int{globalMask=M{};return 1}`, `func erf(x V)V{lanes:=globalMask.ToInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return x};_=mutate();e:=pureLeaf(x);return x.IfElse(globalMask,e)}`, `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`, "arm64", 0},
		{"immutable parameter guard remains valid", ``, `func erf(x V,m M)V{lanes:=m.ToInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return x};e:=pureLeaf(x);return x.IfElse(m,e)}`, `func hot(x V,m M){for i:=0;i<2;i++{_=erf(x,m)}}`, "arm64", 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := prelude + tc.extra + "\n" + tc.helper + "\n" + tc.caller
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "review.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
			sizes := types.SizesFor("gc", tc.arch)
			if sizes == nil {
				t.Fatal("missing target sizes")
			}
			pkg, err := (&types.Config{Sizes: sizes}).Check("simd/archsimd", fset, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostics []analysis.Diagnostic
			pass := &analysis.Pass{Analyzer: PS6123.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(d analysis.Diagnostic) {
				diagnostics = append(diagnostics, d)
				if len(d.SuggestedFixes) != 0 {
					t.Error("unexpected automatic fix")
				}
			}}
			if _, err = runPS6123WithPure(pass, map[string]bool{"simd/archsimd.pureLeaf": true}); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != tc.want {
				t.Errorf("diagnostics=%d want=%d; target=%s\n%s", len(diagnostics), tc.want, tc.arch, source)
			}
		})
	}
}
