package checks

import (
	"strings"
	"testing"
)

const ps6141TwoInputSynthetic = `package fixture
func pack(x []float32) []int8 {p:=make([]int8,len(x));for i:=range x{p[i]=int8(x[i])};return p}
func dot(p,w []int8) int {if len(p)!=len(w){panic("shape")};sum:=0;for i:=range p{sum+=int(p[i])*int(w[i])};return sum}
var sink []int8
func owner(x []float32,w []int8,m,n int) int {
 p:=pack(x)
 return dot(p,w)
}`

func TestPS6141TwoInputDotAdmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"completeSingleDot", "", "", 1},
		{"packedAliasWeight", "dot(p,w)", "dot(p,p)", 0},
		{"weightAlias", "p:=pack(x)", "alias:=w;p:=pack(x)", 0},
		{"sliceWeight", "dot(p,w)", "dot(p,w[:])", 0},
		{"weightRebound", "p:=pack(x)", "w=sink;p:=pack(x)", 0},
		{"packedRetained", "if len(p)", "sink=p;if len(p)", 0},
		{"weightRetained", "if len(p)", "sink=w;if len(p)", 0},
		{"repeatedTraversal", "return sum}", "for i:=range p{sum+=int(p[i])*int(w[i])};return sum}", 0},
		{"MOneOutputRowsReuse", "p:=pack(x)\n return dot(p,w)", "p:=pack(x);m=1;sum:=0;for row:=0;row<n*m;row++{sum+=dot(p,w)};return sum", 0},
		{"consumerOutputRows", "for i:=range p{sum+=int(p[i])*int(w[i])}", "for row:=0;row<2;row++{for i:=range p{sum+=int(p[i])*int(w[i])}}", 0},
		{"callerRepeats", "return dot(p,w)", "dot(p,w);return dot(p,w)", 0},
		{"wrongWeightIndex", "int(w[i])", "int(w[0])", 0},
		{"shapeIgnored", "if len(p)!=len(w)", "if false", 0},
		{"wrongShape", "len(w)", "len(w)+1", 0},
		{"partialTraversal", "range p{", "range p[:1]{", 0},
		{"forwardedUnknown", "return sum}", "return helper(p,w)+sum};func helper(p,w []int8) int{return len(p)+len(w)}", 0},
		{"capture", "return dot(p,w)", "defer func(){sink=p}();return dot(p,w)", 0},
		{"deadBranch", "p:=pack(x)\n return dot(p,w)", "if false{p:=pack(x);return dot(p,w)};return 0", 0},
		{"dynamicRedirection", "return dot(p,w)", "if n>0{return dot(p,w)};return 0", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141TwoInputSynthetic
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == ps6141TwoInputSynthetic {
					t.Fatal("mutation did not change source")
				}
			}
			// Keep an alias negative well-typed and actually used, not an unused
			// declaration that fails before the source proof is exercised.
			if tc.name == "weightAlias" {
				source = strings.ReplaceAll(source, "dot(p,w)", "dot(p,alias)")
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.ConsumerForm = "twoInputDot"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestPS6141TwoInputDotPermutedRolesAndUnsignedStorage(t *testing.T) {
	t.Parallel()
	source := strings.ReplaceAll(ps6141TwoInputSynthetic, "int8", "uint8")
	source = strings.ReplaceAll(source, "func dot(p,w []uint8)", "func dot(w,p []uint8)")
	source = strings.ReplaceAll(source, "dot(p,w)", "dot(w,p)")
	pass, owner := ps6141TypedFixture(t, source)
	c := ps6141TestContract()
	c.ConsumerForm = "twoInputDot"
	c.PackedArgument = 1
	c.WeightArgument = 0
	c.RowsArgument = -1
	if len(ps6141Candidates(pass, owner, &c)) != 1 {
		t.Fatal("complete typed permuted unsigned dot rejected")
	}
}
