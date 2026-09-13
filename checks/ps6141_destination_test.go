package checks

import (
	"strings"
	"testing"
)

// Synthetic codec with a genuine typed fresh-destination call graph, not an
// authentic activation pilot, arithmetic oracle, or benchmark.
func TestPS6141SourceSummaryFreshDestination(t *testing.T) {
	t.Parallel()
	base := strings.ReplaceAll(ps6141LanesSynthetic, "p:=pack(x);return dot(p,w)", "p:=make(Packed,len(x)/32);fill(p,x);return dot(p,w)")
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"positive", "", "", 1},
		{"zeroGeometry", "make(Packed,len(x)/32)", "make(Packed,0)", 0},
		{"truncatedGeometry", "make(Packed,len(x)/32)", "make(Packed,len(x)/64)", 0},
		{"wrongInputGeometry", "make(Packed,len(x)/32)", "make(Packed,len(w)/32)", 0},
		{"differentFloatFormalGeometry", "w Packed,m,n int) float32{p:=make(Packed,len(x)/32)", "w Packed,y []float32,m,n int) float32{p:=make(Packed,len(y)/32)", 0},
		{"wrapperSubstitution", "fill(p,x);return dot(p,w)", "into(x,p);return dot(p,w)};func into(x []float32,p Packed){fill(p,x)", 1},
		{"scratch", "p:=make(Packed,len(x)/32)", "p:=cache", 0},
		{"incomingScratch", "p:=make(Packed,len(x)/32)", "p:=w", 0},
		{"alias", "fill(p,x);return", "q:=p;fill(q,x);return", 0},
		{"retained", "fill(p,x);return", "fill(p,x);cache=p;return", 0},
		{"opaqueGeometry", "len(x)/32);fill", "opaque());fill", 0},
		{"wrongActualDestination", "make(Packed,len(x)/32);fill(p,x);return", "make(Packed,len(x)/32);fill(w,x);return", 0},
		{"helperWrongActual", "fill(p,x);return dot(p,w)", "into(x,p);return dot(p,w)};func into(x []float32,p Packed){fill(cache,x)", 0},
		{"helperFanout", "fill(p,x);return dot(p,w)", "into(x,p);return dot(p,w)};func into(x []float32,p Packed){fill(p,x);opaqueEffect(p)", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := base
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == base {
					t.Fatal("mutation did not change fixture")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			c.ProducerForm = "destination"
			c.Quantizer = "fixture.fill"
			c.DestinationArgument = 0
			c.FloatInputArgument = 1
			if strings.Contains(tc.name, "wrapper") || strings.HasPrefix(tc.name, "helper") {
				c.Quantizer = "fixture.into"
				c.DestinationArgument = 1
				c.FloatInputArgument = 0
			}
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
		})
	}
}
