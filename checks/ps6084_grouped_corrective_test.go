package checks

import "testing"

// These corrective probes preserve the accepted #839 obligations. They are
// synthetic static-analysis cases, not immutable owner replay or benchmarks.
// Every source is independently parsed and typechecked with explicit gc/arm64
// Sizes by the unchanged reviewer-owned helper in architecture_test.go.
func TestPS6084PermanentCorrectiveBoundaries(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		want   int
		source string
	}{
		{name: "ordinary header and dynamic repeat live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "direct initialization loop live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func init(){for i:=range table{table[i]=float32(i)}}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "runtime plain table write silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func mutate(){table[0]=2}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "runtime table increment silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func mutate(){table[0]++}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "table element address escape silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func expose()*float32{return &table[0]}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "table slice alias runtime write silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func mutate(){view:=table[:];view[0]=2}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "init stored writer is not initialization limited", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};var later func();func init(){later=func(){table[0]=2}}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "init asynchronous writer is not initialization limited", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func init(){go func(){table[0]=2}()}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "init synchronous writer live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
var table [256]float32;func decode(v byte)float32{return table[v]};func init(){func(){table[0]=2}()}
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=decode(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "mixed short declaration cannot rewrite prior index", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{idx:=0;c0[idx]=float32(h0[j]);idx,tmp:=j,0;c1[idx]=float32(h1[j])+float32(tmp)};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "mixed short declaration preserves genuinely full live coverage", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{idx:=j;c0[idx]=float32(h0[j]);idx,tmp:=j,0;c1[idx]=float32(h1[j])+float32(tmp)};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "tuple RHS must capture preassignment index", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{idx:=0;idx,prior:=j,idx;c0[prior]=float32(h0[j]);c1[idx]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "tuple index live control", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{idx:=j;idx,prior:=j,idx;c0[prior]=float32(h0[j]);c1[idx]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "disjoint subpath maximum is not complete header", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[8:12];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[12],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "unused part of broad view is not accessed header", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:8];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[8],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "numeric byte native argument is not aggregate carrier", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)
//go:noescape
func scalarLeaf(a,b byte,c,d *float32)(float32,float32)
func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=scalarLeaf(p0[4],p1[4],&c0[0],&c1[0]);}}`},
		{name: "conditional view replacement cannot rewrite actual source", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p2[:4];h1:=p1[:4];if flag{h0=p0[:4]};for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "unconditional stable view live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "one trip inner cannot supply broken outer backedge", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<2;block++{;for once:=0;once<1;once++{for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);};break}}`},
		{name: "one trip inner on truly repeated outer live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<2;block++{;for once:=0;once<1;once++{for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);};}}`},
		{name: "nested synchronous blocking prefix silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{func(){func(){select{}}()}();for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "nested synchronous blocking suffix silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);func(){func(){select{}}()}()}}`},
		{name: "nested synchronous returning prefix live", want: 1, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{func(){func(){return}()}();for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
		{name: "zero enclosing loop cannot borrow inner repeat", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for zero:=0;zero<0;zero++{;for block:=0;block<2;block++{for j:=range 4{c0[j]=float32(h0[j]);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);};}}`},
		{name: "blocking expression in fill remains silent", want: 0, source: `package p
//go:noescape
func leaf(a,b *byte,c,d *float32)(float32,float32)

func run(p0,p1,p2 []byte,trips int,flag bool){var c0,c1 [4]float32;h0:=p0[:4];h1:=p1[:4];for block:=0;block<trips;block++{;for j:=range 4{unused:=<-chan int(nil);c0[j]=float32(h0[j])+float32(unused);c1[j]=float32(h1[j])};_,_=leaf(&p0[4],&p1[4],&c0[0],&c1[0]);}}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ps6084GroupedDiagnostics(t, tc.source)
			if len(got) != tc.want {
				t.Fatalf("grouped diagnostics=%d want=%d: %v", len(got), tc.want, got)
			}
		})
	}
}
