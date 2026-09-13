package checks

import (
	"strings"
	"testing"
)

// Synthetic complete Q8-like value layout and source-call composition only.
// No historical pilot, numerical-equivalence oracle or benchmark is claimed.
const ps6141LanesSynthetic = `package fixture
type Block struct{ Scale float32; Sum int16; Qs [32]int8; Other [32]int8 }
type Packed []Block
var cache Packed
func allocate(n int) Packed{p:=make(Packed,n);return p}
func fill(p Packed,x []float32){
 if len(p)!=len(x)/32{panic("shape")}
 for bi:=range p{
  total:=int16(0)
  for li:=range p[bi].Qs{v:=x[bi*32+li];p[bi].Qs[li]=int8(v*8);total+=int16(v)}
  p[bi].Scale=0.125;p[bi].Sum=total
 }
}
func pack(x []float32) Packed{if len(x)%32!=0{panic("shape")};p:=allocate(len(x)/32);fill(p,x);return p}
func blockDot(p,w Packed) float32{
 if len(p)!=len(w){panic("shape")}
 sum:=float32(0)
 for bi:=range p{
  block:=float32(0)
  for li:=range p[bi].Qs{block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])}
  sum+=block*p[bi].Scale*w[bi].Scale
 }
 return sum
}
func dot(p,w Packed) float32{return blockDot(p,w)}
func owner(x []float32,w Packed,m,n int) float32{p:=pack(x);return dot(p,w)}
func opaque() int
func opaqueEffect(p Packed)
func clear(v float32) float32{v=0;return v}
`

func TestPS6141SourceSummaryAssociatedBlockLanes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"completeMultiCallQ8LikeLayout", "", "", 1},
		{"extraConsumerWrapper", "return blockDot(p,w)", "return wrapper(p,w)};func wrapper(p,w Packed) float32{return blockDot(p,w)", 1},
		{"producerCodecPrepass", "p[bi].Scale=0.125", "for li:=range p[bi].Qs{total+=int16(x[bi*32+li])};p[bi].Scale=0.125", 1},
		{"explicitAdditiveLaneRecurrence", "block+=float32(p[bi].Qs[li])", "block=block+float32(p[bi].Qs[li])", 1},
		{"explicitAdditiveBlockRecurrence", "sum+=block*p[bi].Scale", "sum=sum+block*p[bi].Scale", 1},
		{"returnedScalarAlias", "return sum", "saved:=sum;sum=0;return saved", 1},
		{"lastLaneOverwrite", "block+=float32(p[bi].Qs[li])", "block=float32(p[bi].Qs[li])", 0},
		{"lastBlockOverwrite", "sum+=block*p[bi].Scale", "sum=block*p[bi].Scale", 0},
		{"finalAccumulatorReset", "return sum", "sum=0;return sum", 0},
		{"producerTerminalPanic", "fill(p,x);return p", "panic(\"stop\");fill(p,x);return p", 0},
		{"consumerTerminalPanic", "sum:=float32(0)", "panic(\"stop\");sum:=float32(0)", 0},
		{"producerUnreachableFillAfterReturn", "fill(p,x);return p", "return p;fill(p,x);return p", 0},
		{"helperErasesReturnedReduction", "return sum", "sum=clear(sum);return sum", 0},
		{"overwrittenCompletedLaneReduction", "block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])", "block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li]);block=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])", 0},
		{"overwrittenCompletedBlockReduction", "sum+=block*p[bi].Scale*w[bi].Scale", "sum+=block*p[bi].Scale*w[bi].Scale;sum=block*p[bi].Scale*w[bi].Scale", 0},
		{"wrongPackedBlock", "float32(p[bi].Qs[li])", "float32(p[0].Qs[li])", 0},
		{"wrongWeightBlock", "float32(w[bi].Qs[li])", "float32(w[bi+1].Qs[li])", 0},
		{"wrongLane", "float32(w[bi].Qs[li])", "float32(w[bi].Qs[li+1])", 0},
		{"constantLane", "float32(p[bi].Qs[li])", "float32(p[bi].Qs[0])", 0},
		{"wrongFloatBlockExtent", "x[bi*32+li]", "x[bi*16+li]", 0},
		{"wrongFloatLane", "x[bi*32+li]", "x[bi*32+li+1]", 0},
		{"wrongWriteLane", "p[bi].Qs[li]=int8", "p[bi].Qs[0]=int8", 0},
		{"wrongArrayExtent", "Qs [32]int8", "Qs [16]int8", 0},
		{"producerBoundsUnknown", "if len(p)!=len(x)/32", "if false", 0},
		{"overflowingProductBounds", "if len(p)!=len(x)/32", "if len(x)!=len(p)*32", 0},
		{"wrongProducerQuotient", "if len(p)!=len(x)/32", "if len(p)!=len(x)/16", 0},
		{"consumerBoundsUnknown", "if len(p)!=len(w)", "if false", 0},
		{"consumerWrongBounds", "if len(p)!=len(w)", "if len(p)!=len(w)*2", 0},
		{"wrongArrayField", "range p[bi].Qs", "range p[bi].Other", 0},
		{"wrongInnerBlock", "range p[bi].Qs", "range p[0].Qs", 0},
		{"wrongInnerRoot", "for li:=range p[bi].Qs{block+=", "for li:=range w[bi].Qs{block+=", 0},
		{"outerIndexWrite", "for li:=range p[bi].Qs{block+=", "for li:=range p[bi].Qs{bi=0;block+=", 0},
		{"laneIndexWrite", "for li:=range p[bi].Qs{block+=", "for li:=range p[bi].Qs{li=0;block+=", 0},
		{"producerLaneIndexWrite", "for li:=range p[bi].Qs{v:=", "for li:=range p[bi].Qs{li=0;v:=", 0},
		{"producerOuterIndexWrite", "for li:=range p[bi].Qs{v:=", "for li:=range p[bi].Qs{bi=0;v:=", 0},
		{"capturedLane", "for li:=range p[bi].Qs{block+=", "for li:=range p[bi].Qs{defer func(){_ = li}();block+=", 0},
		{"unknownTrip", "for li:=range p[bi].Qs{block+=", "for li:=0;li<opaque();li++{block+=", 0},
		{"outputRowFanout", "for li:=range p[bi].Qs{block+=", "for li:=range p{block+=", 0},
		{"opaqueArrayIndex", "p[bi].Qs[li]", "p[opaque()].Qs[li]", 0},
		{"opaqueEffect", "sum+=block*p[bi].Scale*w[bi].Scale", "opaqueEffect(p);sum+=block*p[bi].Scale*w[bi].Scale", 0},
		{"opaqueStorage", "p:=allocate(len(x)/32)", "p:=cache", 0},
		{"helperFanout", "return blockDot(p,w)", "return blockDot(p,w)+blockDot(p,w)", 0},
		{"multipleOutputs", "sum+=block*p[bi].Scale*w[bi].Scale", "sum+=block*p[bi].Scale*w[bi].Scale;sum+=block", 0},
		{"independentInnerOutputs", "for li:=range p[bi].Qs{block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])}", "other:=float32(0);for li:=range p[bi].Qs{block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li]);other+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])};block+=other", 0},
		{"repeatedLaneTraversal", "sum+=block*p[bi].Scale*w[bi].Scale", "for li:=range p[bi].Qs{block+=float32(p[bi].Qs[li])*float32(w[bi].Qs[li])};sum+=block*p[bi].Scale*w[bi].Scale", 0},
		{"repeatedFullPackedTraversal", "return sum", "for bi:=range p{sum+=p[bi].Scale*w[bi].Scale};return sum", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141LanesSynthetic
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == ps6141LanesSynthetic {
					t.Fatal("mutation did not change exact fixture")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
		})
	}
}
