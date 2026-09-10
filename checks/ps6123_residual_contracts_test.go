package checks

import "testing"

// Residual witnesses only for the already accepted same-path Range backedge
// and exact SDK callable-object obligations. The retained 67 remain untouched.
func TestPS6123PermanentResidualContracts(t *testing.T) {
	t.Parallel()
	const common = `package p
type Float64x2 struct{};type Mask64x2 struct{};type Int64x2 struct{}
type V=Float64x2;type M=Mask64x2;type I=Int64x2
func(V)Mul(V)V{return V{}};func(V)Div(V)V{return V{}};func(V)MulAdd(V,V)V{return V{}}
func(V)IfElse(M,V)V{return V{}};func(V)Less(V)M{return M{}}
func(M)ToInt64x2()I{return I{}};func(I)GetElem(uint8)int64{return 0}
func pureLeaf(V)V{return V{}}
`
	const selectField = `package p
type Float64x2 struct{IfElse func(Mask64x2,Float64x2)Float64x2};type Mask64x2 struct{}
type V=Float64x2;type M=Mask64x2
func(V)Less(V)M{return M{}}
func pureLeaf(V)V{return V{}}
`
	const promotedMethod = `package p
type Evil struct{};type Float64x2 struct{Evil};type Mask64x2 struct{}
type V=Float64x2;type M=Mask64x2
var effects int
func(Evil)Mul(v V)V{effects++;return v}
func(V)IfElse(M,V)V{return V{}};func(V)Less(V)M{return M{}}
func pureLeaf(V)V{return V{}}
`
	const helper = `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);return x.IfElse(m,e)}`
	const mulHelper = `func erf(x V)V{m:=x.Less(x);e:=pureLeaf(x);e=x.Mul(e);return x.IfElse(m,e)}`
	const repeated = `func hot(x V){for i:=0;i<2;i++{_=erf(x)}}`
	cases := []struct {
		name, prelude, helper, caller string
		want                          int
	}{
		{"switch case candidate breaks outer range", common, helper, `func hot(x V,flag bool){outer:for range [2]int{}{switch flag{case true:for i:=0;i<1;i++{_=erf(x)};break outer;default:}}}`, 0},
		{"switch case candidate actually repeats", common, helper, `func hot(x V,flag bool){outer:for range [2]int{}{switch flag{case true:for i:=0;i<1;i++{_=erf(x)};if false{break outer};default:}}}`, 1},
		{"IfElse field is not canonical select method", selectField, helper, repeated, 0},
		{"promoted Mul must use its actual method receiver", promotedMethod, mulHelper, repeated, 0},
		{"canonical direct Mul and IfElse live", common, mulHelper, repeated, 1},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := tc.prelude + "\n" + tc.helper + "\n" + tc.caller
			got := ps6123PermanentDiagnosticsWithPure(t, source, map[string]bool{"simd/archsimd.pureLeaf": true})
			if got != tc.want {
				t.Fatalf("diagnostics=%d want=%d\n%s", got, tc.want, source)
			}
		})
	}
}
