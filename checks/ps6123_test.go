package checks

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS6123ValueVersionsNestedSelectsAndUniformGuard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, guard, caller string
		want                int
	}{
		{"ownerV1NestedRebinding", "", `func hot(dst,src []V){if !eligible(src){return};n2:=len(src)&^1;for i:=0;i<n2;i+=2{dst[i]=erf(src[i])};for i:=n2;i<len(src);i++{dst[i]=src[i]}};func hotGrad(dst,x,g []V){if !eligible(x){return};n2:=len(x)&^1;for i:=0;i<n2;i+=2{phi:=erf(x[i]);dst[i]=g[i].Mul(phi)};for i:=n2;i<len(x);i++{dst[i]=g[i]}}`, 2},
		{"ownerV2InnerGuard", `lanes:=small.ToInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return erfSmall}`, `func hot(dst,src []V){if !eligible(src){return};n2:=len(src)&^1;for i:=0;i<n2;i+=2{dst[i]=erf(src[i])}}`, 1},
		{"noRepeatedCaller", "", `func cold(x V)V{return erf(x)}`, 0},
		{"deadRepeatedCaller", "", `func hot(dst,src []V){for i:=0;i<len(src);i++{if false{dst[i]=erf(src[i])}}}`, 0},
		{"oneTripCaller", "", `func hot(dst,src []V){for i:=0;i<1;i++{dst[i]=erf(src[i])}}`, 0},
		{"zeroRangeCaller", "", `func hot(dst,src []V){for i:=range [0]int{}{dst[i]=erf(src[i])}}`, 0},
		{"twoTripCaller", "", `func hot(dst,src []V){for i:=0;i<2;i++{dst[i]=erf(src[i])}}`, 2},
		{"hotThenColdCaller", "", `func hot(dst,src []V){for i:=0;i<2;i++{dst[i]=erf(src[i])};for i:=0;i<1;i++{dst[i]=erf(src[i])}}`, 2},
		{"coldThenHotCaller", "", `func hot(dst,src []V){for i:=0;i<1;i++{dst[i]=erf(src[i])};for i:=0;i<2;i++{dst[i]=erf(src[i])}}`, 2},
		{"unrelatedPostRepeats", "", `func hot(dst,src []V){j:=0;for i:=0;i<1;j++{dst[0]=erf(src[0]);if j>2{return}}}`, 2},
		{"bodyWriteStopsRepeat", "", `func hot(dst,src []V){for i:=0;i<2;i++{dst[0]=erf(src[0]);i=2}}`, 0},
		{"breakStopsRepeat", "", `func hot(dst,src []V){for i:=0;i<2;i++{dst[0]=erf(src[0]);break}}`, 0},
		{"uint8WrapOneTrip", "", `func hot(dst,src []V){for i:=uint8(255);i>0;i++{dst[0]=erf(src[0])}}`, 0},
		{"deferredCaller", "", `func hot(src []V){for i:=range src{defer func(){_=erf(src[i])}()}}`, 0},
		{"partialMaskGuard", `lanes:=small.ToInt64x2();if lanes.GetElem(0)!=0{return erfSmall}`, `func hot(dst,src []V){for i:=range src{dst[i]=erf(src[i])}}`, 2},
		{"anyLaneGuard", `lanes:=small.ToInt64x2();if lanes.GetElem(0)!=0||lanes.GetElem(1)!=0{return erfSmall}`, `func hot(dst,src []V){for i:=range src{dst[i]=erf(src[i])}}`, 2},
		{"wrongArmGuard", `lanes:=small.ToInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return y}`, `func hot(dst,src []V){for i:=range src{dst[i]=erf(src[i])}}`, 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := `package p
type Float64x2 struct{}; type Mask64x2 struct{}; type Int64x2 struct{}
type V = Float64x2; type M = Mask64x2; type I = Int64x2
func(V)Abs()V{return V{}};func(V)Add(V)V{return V{}};func(V)Sub(V)V{return V{}};func(V)MulAdd(V,V)V{return V{}};func(V)Mul(V)V{return V{}};func(V)Div(V)V{return V{}}
func(V)Less(V)M{return M{}};func(V)GreaterEqual(V)M{return M{}};func(V)IfElse(M,V)V{return V{}}
func(M)ToInt64x2()I{return I{}};func(I)GetElem(uint8)int64{return 0}
func expensive(V)V{return V{}};func eligible([]V)bool{return true}
func erf(y V)V{ay:=y.Abs();z:=y.Mul(y);numT:=y.MulAdd(z,y);numT=numT.MulAdd(z,y);numT=numT.MulAdd(z,y);numT=numT.MulAdd(z,y);denU:=z.Add(y);denU=denU.MulAdd(z,y);denU=denU.MulAdd(z,y);denU=denU.MulAdd(z,y);denU=denU.MulAdd(z,y);erfSmall:=y.Mul(numT).Div(denU);small:=ay.Less(y);` + tc.guard + `;e:=expensive(z);numP:=y.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);numP=numP.MulAdd(ay,y);denQ:=ay.Add(y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);denQ=denQ.MulAdd(ay,y);erfc:=e.Mul(numP).Div(denQ);erfMiddle:=y.Sub(erfc);erfBig:=y;erf:=erfSmall.IfElse(small,erfMiddle);return erfBig.IfElse(ay.GreaterEqual(y),erf)}
` + tc.caller
			if got := ps6123Diagnostics(t, source); got != tc.want {
				t.Fatalf("diagnostics=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestPS6123MetadataAndRejectedEvidence(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS6123.Doc.Text+PS6123.Doc.MeasuredWin), " ")
	for _, fragment := range []string{"NO automatic fix", "V1", "V2", "REJECTED", "68.95–76.65%", "2.058–3.924x", "not V1 to V2", "signed zero", "NaN/Inf", "mixed-lane", "complete operation"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("missing %q", fragment)
		}
	}
	if PS6123.AutoFix || !PS6123.NeedsConfig || PS6123.Level != 3 {
		t.Fatalf("metadata drift: %+v", PS6123)
	}
}

type ps6123Importer struct{ arch *types.Package }

func (value ps6123Importer) Import(path string) (*types.Package, error) {
	if path == "simd/archsimd" {
		return value.arch, nil
	}
	return importer.Default().Import(path)
}

func TestPS6123CompletePinnedOwnerSources(t *testing.T) {
	t.Parallel()
	archSource := `package archsimd
type Float64x2 struct{};type Mask64x2 struct{};type Int64x2 struct{};type Uint64x2 struct{}
func BroadcastFloat64x2(float64)Float64x2;func BroadcastInt64x2(int64)Int64x2;func BroadcastUint64x2(uint64)Uint64x2;func LoadFloat64x2Array(*[2]float64)Float64x2
func(Float64x2)Max(Float64x2)Float64x2;func(Float64x2)Mul(Float64x2)Float64x2;func(Float64x2)Round()Float64x2;func(Float64x2)MulAdd(Float64x2,Float64x2)Float64x2;func(Float64x2)Add(Float64x2)Float64x2;func(Float64x2)Sub(Float64x2)Float64x2;func(Float64x2)Div(Float64x2)Float64x2;func(Float64x2)Abs()Float64x2;func(Float64x2)ConvertToInt64()Int64x2;func(Float64x2)ToBits()Uint64x2;func(Float64x2)Less(Float64x2)Mask64x2;func(Float64x2)GreaterEqual(Float64x2)Mask64x2;func(Float64x2)IfElse(Mask64x2,Float64x2)Float64x2;func(Float64x2)StoreArray(*[2]float64)
func(Mask64x2)ToInt64x2()Int64x2;func(Int64x2)GetElem(uint8)int64;func(Int64x2)Add(Int64x2)Int64x2;func(Int64x2)ShiftAllLeft(uint8)Int64x2;func(Int64x2)ToBits()Uint64x2;func(Uint64x2)And(Uint64x2)Uint64x2;func(Uint64x2)Or(Uint64x2)Uint64x2;func(Uint64x2)BitsToFloat64()Float64x2`
	fset := token.NewFileSet()
	sizes := types.SizesFor("gc", "arm64")
	archFile, err := parser.ParseFile(fset, "arch.go", archSource, 0)
	if err != nil {
		t.Fatal(err)
	}
	archPkg, err := (&types.Config{Sizes: sizes}).Check("simd/archsimd", fset, []*ast.File{archFile}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stub := `package cpu
const cpuInvSqrt2Pi=0.3989422804014327
func expF64poly(float64)float64{return 0};func geluGradF64(float64,float64)float64{return 0}`
	for _, tc := range []struct {
		name, path string
		want       int
	}{{"V1", "testdata/ps6123_owner_v1.go", 2}, {"V2", "testdata/ps6123_owner_v2.go", 1}} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			for _, caller := range []string{"vgeluF64", "vgeluGradF64"} {
				caller := caller
				t.Run(caller, func(t *testing.T) {
					t.Parallel()
					localSet := token.NewFileSet()
					owner, err := parser.ParseFile(localSet, tc.path, data, parser.ParseComments)
					if err != nil {
						t.Fatal(err)
					}
					filtered := owner.Decls[:0]
					for _, declaration := range owner.Decls {
						if fn, ok := declaration.(*ast.FuncDecl); ok && (fn.Name.Name == "vgeluF64" || fn.Name.Name == "vgeluGradF64") && fn.Name.Name != caller {
							continue
						}
						filtered = append(filtered, declaration)
					}
					owner.Decls = filtered
					extra, err := parser.ParseFile(localSet, "stub.go", stub, 0)
					if err != nil {
						t.Fatal(err)
					}
					info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
					pkg, err := (&types.Config{Importer: ps6123Importer{archPkg}, Sizes: sizes}).Check("github.com/jxsl13/goai/backend/cpu", localSet, []*ast.File{owner, extra}, info)
					if err != nil {
						t.Fatal(err)
					}
					count := 0
					pass := &analysis.Pass{Analyzer: PS6123.Analyzer, Fset: localSet, Files: []*ast.File{owner, extra}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(analysis.Diagnostic) { count++ }}
					if _, err := runPS6123WithPure(pass, map[string]bool{"github.com/jxsl13/goai/backend/cpu.expF64x2GELU": true}); err != nil {
						t.Fatal(err)
					}
					if count != tc.want {
						t.Fatalf("diagnostics=%d, want %d", count, tc.want)
					}
				})
			}
		})
	}
}

func TestPS6123ExclusiveDAGAndGuardOrdering(t *testing.T) {
	t.Parallel()
	const prelude = `package p
type Float64x2 struct{};type Mask64x2 struct{};type Int64x2 struct{};type V=Float64x2;type M=Mask64x2;type I=Int64x2
func(V)Mul(V)V{return V{}};func(V)Div(V)V{return V{}};func(V)IfElse(M,V)V{return V{}};func(V)Less(V)M{return M{}};func(M)ToInt64x2()I{return I{}};func(M)FakeInt64x2()I{return I{}};func(I)GetElem(uint8)int64{return 0};func expensive(V)V{return V{}};var configuredVar func(V)V
`
	cases := []struct {
		name, helper string
		want         int
	}{
		{"sharedExpensiveSilent", `func erf(x V)V{m:=x.Less(x);e:=expensive(x);a:=e.Mul(e);b:=e;return a.IfElse(m,b)}`, 0},
		{"exclusiveExpensiveReports", `func erf(x V)V{m:=x.Less(x);e:=expensive(x);return x.IfElse(m,e)}`, 1},
		{"equalDistinctExclusiveSilent", `func erf(x V)V{m:=x.Less(x);a:=expensive(x);b:=expensive(x);return a.IfElse(m,b)}`, 0},
		{"configuredFunctionVariableSilent", `func erf(x V)V{m:=x.Less(x);a:=configuredVar(x);return x.IfElse(m,a)}`, 0},
		{"fakeMaskConversionDoesNotSuppress", `func erf(x V)V{m:=x.Less(x);lanes:=m.FakeInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return x};a:=expensive(x);return x.IfElse(m,a)}`, 1},
		{"lateGuardStillReports", `func erf(x V)V{m:=x.Less(x);small:=x;e:=expensive(x);lanes:=m.ToInt64x2();if lanes.GetElem(0)!=0&&lanes.GetElem(1)!=0{return small};return small.IfElse(m,e)}`, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := prelude + tc.helper + `;func hot(dst,src []V){for i:=0;i<len(src);i+=2{dst[i]=erf(src[i])}}`
			if got := ps6123Diagnostics(t, source); got != tc.want {
				t.Fatalf("diagnostics=%d, want %d", got, tc.want)
			}
		})
	}
}

func ps6123Diagnostics(t *testing.T, source string) int {
	t.Helper()
	fset := token.NewFileSet()
	sizes := types.SizesFor("gc", "arm64")
	file, err := parser.ParseFile(fset, "owner.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Instances: map[*ast.Ident]types.Instance{}}
	pkg, err := (&types.Config{Sizes: sizes}).Check("simd/archsimd", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	pass := &analysis.Pass{Analyzer: PS6123.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: sizes, Report: func(d analysis.Diagnostic) {
		if len(d.SuggestedFixes) != 0 {
			t.Error("unexpected fix")
		}
		count++
	}}
	if _, err := runPS6123WithPure(pass, map[string]bool{"simd/archsimd.expensive": true}); err != nil {
		t.Fatal(err)
	}
	return count
}
