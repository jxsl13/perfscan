package checks

import (
	"fmt"
	"testing"
)

// Independent bounded probes of only the newly added table-borrow region proof.
func TestPS6084PermanentOpaqueBorrowCallbacks(t *testing.T) {
	t.Parallel()
	const prelude = `package p
%s
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
//go:noescape
func borrow(*float32)
var table [256]float32
func init(){for i:=range table{table[i]=float32(i)}}
func decode(v byte)float32{return table[v]}
`
	const wrapper = `func run(p0,p1 []byte,trips int){%s;for block:=0;block<trips;block++{var c0,c1 [4]float32;for j:=range 4{c0[j]=decode(p0[j]);c1[j]=decode(p1[j])};_,_=leaf(&p0[0],&p1[0],&c0[0],&c1[0])}}`
	for _, tc := range []struct {
		name, imports, declaration, prefix string
		want                               int
	}{
		{"opaque callback dispatcher", "", "func dispatch(func())", "dispatch(func(){borrow(&table[0])})", 0},
		{"noescape callback dispatcher", "", "//go:noescape\nfunc dispatch(func())", "dispatch(func(){borrow(&table[0])})", 0},
		{"imported callback dispatcher", `import "sort"`, "", "sort.Slice([]int{2,1},func(i,j int)bool{borrow(&table[0]);return i<j})", 0},
		{"uninvoked literal remains separate", "", "", "unused:=func(){borrow(&table[0])};_=unused", 1},
		{"direct invoked literal remains visible", "", "", "func(){borrow(&table[0])}()", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := fmt.Sprintf(prelude, tc.imports) + tc.declaration + "\n" + fmt.Sprintf(wrapper, tc.prefix)
			if got := ps6084GroupedDiagnostics(t, source); len(got) != tc.want {
				t.Fatalf("diagnostics=%d want=%d: %v", len(got), tc.want, got)
			}
		})
	}
}
