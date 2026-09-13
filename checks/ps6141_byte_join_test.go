package checks

import (
	"go/types"
	"os"
	"strings"
	"testing"
)

// Mixed evidence: authentic model-weight codec source plus a SYNTHETIC owner
// and scalar byte consumer. This is not an observed activation-positive pilot,
// a Q8 numerical-equivalence oracle, or a benchmark.
func TestPS6141AuthenticCodecSyntheticCheckedBoundary(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/ps6141_authentic_q8_0.go")
	if err != nil {
		t.Fatal(err)
	}
	harness := `func owner(x []float32,w []byte,y []float32) float32{if len(x)/32>1024{panic("shape")};p:=quantizeQ8_0(x);return dot(p,w)}
func dot(p,w []byte)float32{return leaf(p,w)}
func leaf(p,w []byte)float32{if len(p)!=len(w){panic("shape")};sum:=float32(0);for i:=range p{sum+=float32(p[i])*float32(w[i])};return sum}
var byteCache []byte
func opaqueLimit() int
`
	base := strings.Replace(string(raw), "func owner(x []float32) []byte { return quantizeQ8_0(x) }", harness, 1)
	if base == string(raw) {
		t.Fatal("synthetic owner join did not change exact scaffold")
	}
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"genuineMixedPrerequisite", "", "", 1},
		{"noGeometryGuard", "if len(x)/32>1024{panic(\"shape\")};", "", 0},
		{"gotoBypassesGuard", "if len(x)/32>1024{panic(\"shape\")};p:=", "goto Pack;if len(x)/32>1024{panic(\"shape\")};Pack:p:=", 0},
		{"conditionalGotoBypassesGuard", "if len(x)/32>1024{panic(\"shape\")};p:=", "if len(y)>0{goto Pack};if len(x)/32>1024{panic(\"shape\")};Pack:p:=", 0},
		{"wrongInputGuard", "len(x)/32>1024", "len(y)/32>1024", 0},
		{"wrongExtentGuard", "len(x)/32>1024", "len(x)/64>1024", 0},
		{"overflowingLimit", "len(x)/32>1024", "len(x)/32>9223372036854775807", 0},
		{"zeroOnlyGuard", "len(x)/32>1024", "len(x)/32>0", 0},
		{"opaqueLimit", "len(x)/32>1024", "len(x)/32>opaqueLimit()", 0},
		{"recoveredStoragePayload", "panic(\"shape\")};p:=", "panic(x)};p:=", 0},
		{"reboundInput", "p:=quantizeQ8_0(x)", "x=y;p:=quantizeQ8_0(x)", 0},
		{"aliasedPacked", "return dot(p,w)", "q:=p;return dot(q,w)", 0},
		{"cachedPacked", "return dot(p,w)", "byteCache=p;return dot(p,w)", 0},
		{"secondConsumer", "return dot(p,w)", "dot(p,w);return dot(p,w)", 0},
		{"NOutputReuse", "return leaf(p,w)", "sum:=float32(0);for row:=range 2{_ = row;sum+=leaf(p,w)};return sum", 0},
		{"repeatedPackedTraversal", "return sum}", "for i:=range p{sum+=float32(p[i])*float32(w[i])};return sum}", 0},
		{"consumerIgnoresReduction", "return sum}", "return float32(int(sum)&0)}", 0},
		{"producerIgnoresLaneArgument", "return float32(math.Round(float64(v)))", "return 0", 0},
		{"producerRebindsLaneValue", "for j, v := range blk {", "for j, v := range blk {v=0;", 0},
		{"deadLaneFill", "out[o+2+j] = byte(int8(roundHalfAway(v * id)))", "if false{out[o+2+j] = byte(int8(roundHalfAway(v * id)))}", 0},
		{"conditionalLaneFill", "out[o+2+j] = byte(int8(roundHalfAway(v * id)))", "if v!=0{out[o+2+j] = byte(int8(roundHalfAway(v * id)))}", 0},
		{"unknownTargetSizes", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := base
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == base {
					t.Fatal("mutation did not change exact fixture")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			if tc.name == "unknownTargetSizes" {
				pass.TypesSizes = nil
			}
			c := ps6141TestContract()
			c.Quantizer = "fixture.quantizeQ8_0"
			c.Consumer = "fixture.dot"
			c.ConsumerForm = "sourceSummary"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
			if tc.name == "genuineMixedPrerequisite" {
				declarations := ps6099LocalFunctionDeclarations(pass)
				index := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
				for fn := range declarations {
					if fn.Name() == "dot" {
						s := index.function(fn)
						if !s.valid || s.traversals[1] != 1 || !s.resultReductions[1] || !s.resultDeps[2] {
							t.Fatal("independent whole consumer prerequisite failed")
						}
					}
				}
			}
		})
	}
}
