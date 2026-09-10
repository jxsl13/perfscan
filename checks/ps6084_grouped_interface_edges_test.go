package checks

import (
	"fmt"
	"testing"
)

func TestPS6084IndependentInterfaceCallbackEdges(t *testing.T) {
	t.Parallel()
	const prelude = `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
//go:noescape
func borrow(*float32)
var table [256]float32
func decode(v byte)float32{return table[v]}
type Caller interface{Call()}
type V struct{}
func(V)Call(){borrow(&table[0])}
type Box struct{Caller}
`
	const wrapper = `func run(p0,p1 []byte,trips int){%s;for block:=0;block<trips;block++{var c0,c1 [4]float32;for j:=range 4{c0[j]=decode(p0[j]);c1[j]=decode(p1[j])};_,_=leaf(&p0[0],&p1[0],&c0[0],&c1[0])}}`
	for _, tc := range []struct {
		name, declaration, prefix string
		want                      int
	}{
		{"interface method value callback", "func dispatch(func())", "var x Caller=V{};dispatch(x.Call)", 0},
		{"promoted interface direct call", "", "x:=Box{V{}};x.Call()", 0},
		{"promoted interface method expression", "", "x:=Box{V{}};Box.Call(x)", 0},
		{"promoted interface method callback", "func dispatch(func())", "x:=Box{V{}};dispatch(x.Call)", 0},
		{"concrete primitive method live", "func(V) Noop(){};func dispatch(func())", "v:=V{};dispatch(v.Noop)", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := prelude + tc.declaration + "\n" + fmt.Sprintf(wrapper, tc.prefix)
			if got := ps6084GroupedDiagnostics(t, source); len(got) != tc.want {
				t.Fatalf("diagnostics=%d want=%d: %v", len(got), tc.want, got)
			}
		})
	}
}
