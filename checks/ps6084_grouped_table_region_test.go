package checks

import (
	"fmt"
	"testing"
)

func TestPS6084GroupedTableRegionStability(t *testing.T) {
	t.Parallel()
	const prelude = `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
//go:noescape
func borrow(*float32)
var table [256]float32
func init(){for i:=range table{table[i]=float32(i)}}
func decode(v byte)float32{return table[v]}
`
	const run = `func run(p0,p1 []byte,trips int){%s;for block:=0;block<trips;block++{var c0,c1 [4]float32;for j:=range 4{c0[j]=decode(p0[j]);c1[j]=decode(p1[j])};_,_=leaf(&p0[0],&p1[0],&c0[0],&c1[0])}}`
	cases := []struct {
		name, declarations, prefix string
		want                       int
	}{
		{"separate synchronous noescape borrow", `func other(){borrow(&table[0])}`, ``, 1},
		{"same region noescape borrow", ``, `borrow(&table[0])`, 0},
		{"reachable asynchronous borrow", `func other(){borrow(&table[0])}`, `go other()`, 0},
		{"indirect reachability is conservative", `func other(){borrow(&table[0])};var invoke=other`, `invoke()`, 0},
		{"retained address remains an escape", `func expose()*float32{return &table[0]}`, ``, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := prelude + tc.declarations + "\n" + fmt.Sprintf(run, tc.prefix)
			got := ps6084GroupedDiagnostics(t, source)
			if len(got) != tc.want {
				t.Fatalf("grouped diagnostics=%d want=%d: %v", len(got), tc.want, got)
			}
		})
	}
}
