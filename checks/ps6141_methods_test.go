package checks

import (
	"go/types"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// A source-visible weight/shape receiver modeled on QuantLinear's API shape.
// This controlled activation owner is synthetic, not a historical pilot.
const ps6141MethodSynthetic = `package fixture
type Block struct { Q int8; Scale float32; Sum int16 }
type Packed []Block
type Kernel struct { Weight Packed; In, Out int }
var other *Kernel
func storage(n int) Packed {p:=make(Packed,n);return p}
func fill(p Packed,x []float32) {for i:=range x {p[i].Scale=x[i]*0.125;p[i].Q=int8(x[i]*8);p[i].Sum=int16(x[i])+3}}
func pack(x []float32) Packed {p:=storage(len(x));fill(p,x);return p}
func leaf(p,w Packed) float32 {if len(p)!=len(w){panic("shape")};sum:=float32(0);for i:=range p {sum+=float32(p[i].Q)*float32(w[i].Q)*p[i].Scale*w[i].Scale};return sum}
func (q *Kernel) reduce(p Packed) float32 {return leaf(p,q.Weight)}
func (q *Kernel) dot(p Packed) float32 {return q.reduce(p)}
func owner(x []float32,q *Kernel) float32 {p:=pack(x);return q.dot(p)}
`

func TestPS6141SourceMethodReceiverComposition(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, replacement string
		want                   int
	}{
		{"sourcePointerWeightReceiver", "", "", 1},
		{"sourceValueWeightReceiver", "*Kernel", "Kernel", 1},
		{"directReceiverFieldForward", "return q.reduce(p)", "return leaf(p,q.Weight)", 1},
		{"ownerReceiverAlias", "p:=pack(x);return q.dot(p)", "alias:=q;p:=pack(x);return alias.dot(p)", 0},
		{"ownerReceiverRebind", "p:=pack(x);return q.dot(p)", "q=other;p:=pack(x);return q.dot(p)", 0},
		{"ownerReceiverExposure", "p:=pack(x);return q.dot(p)", "expose(q);p:=pack(x);return q.dot(p)", 0},
		{"helperReceiverAlias", "return q.reduce(p)", "alias:=q;return alias.reduce(p)", 0},
		{"differentReceiverInvocation", "return q.reduce(p)", "return other.reduce(p)", 0},
		{"receiverReboundInMethod", "return q.reduce(p)", "q=other;return q.reduce(p)", 0},
		{"receiverFieldMutation", "return q.reduce(p)", "q.Weight[0].Q=0;return q.reduce(p)", 0},
		{"receiverFieldRebind", "return q.reduce(p)", "q.Weight=p;return q.reduce(p)", 0},
		{"receiverShapeMutation", "return q.reduce(p)", "q.In++;return q.reduce(p)", 0},
		{"retainedPackedAlias", "return q.reduce(p)", "q.Weight=p;return q.reduce(p)", 0},
		{"differentPackedInvocation", "return q.reduce(p)", "return q.reduce(q.Weight)", 0},
		{"multipleMethodUses", "return q.reduce(p)", "return q.reduce(p)+q.reduce(p)", 0},
		{"multipleOwnerUses", "return q.dot(p)", "q.dot(p);return q.dot(p)", 0},
		{"receiverFieldPassedTwice", "return leaf(p,q.Weight)", "return leaf(q.Weight,q.Weight)", 0},
		{"receiverStorageEscape", "return q.reduce(p)", "expose(q);return q.reduce(p)", 0},
		{"opaqueMethod", "func (q *Kernel) reduce(p Packed) float32 {return leaf(p,q.Weight)}", "func (q *Kernel) reduce(p Packed) float32", 0},
		{"methodValueCapture", "return q.reduce(p)", "f:=q.reduce;return f(p)", 0},
		{"methodExpression", "return q.reduce(p)", "return (*Kernel).reduce(q,p)", 0},
		{"memoryBearingCacheField", "In, Out int", "In, Out int; Cache *Block", 0},
		{"secondStorageField", "In, Out int", "In, Out int; Scratch Packed", 0},
		{"missingSourceWeightOrigin", "return leaf(p,q.Weight)", "return shapeOnly(p,q.Out)", 0},
		{"errorReturningMethod", "float32 {return q.reduce(p)}", "(float32,error) {return q.reduce(p),nil}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141MethodSynthetic + `func expose(q *Kernel){}
func shapeOnly(p Packed,n int)float32{sum:=float32(0);for i:=range p{sum+=float32(p[i].Q)*float32(n)};return sum}
`
			if tc.old != "" {
				changed := strings.ReplaceAll(source, tc.old, tc.replacement)
				if changed == source {
					t.Fatal("adversarial source mutation did not execute")
				}
				source = changed
			}
			if tc.name == "errorReturningMethod" {
				source = strings.Replace(source, "return q.dot(p)", "v,_:=q.dot(p);return v", 1)
			}
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.Consumer = "fixture.Kernel.dot"
			c.ConsumerForm, c.WeightArgument, c.RowsArgument = "sourceSummary", -1, -1
			if !c.Valid() {
				t.Fatal("method-role contract prerequisite rejected")
			}
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
			var diagnostics []analysis.Diagnostic
			pass.Report = func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }
			if _, err := runPS6141WithContracts(pass, []config.SingleUseQuantizationContract{c}); err != nil || len(diagnostics) != tc.want {
				t.Fatalf("advisories=%d want=%d err=%v", len(diagnostics), tc.want, err)
			}
			for _, diagnostic := range diagnostics {
				if len(diagnostic.Related) != 1 || len(diagnostic.SuggestedFixes) != 0 {
					t.Fatalf("method advisory lost source boundary or added a rewrite: %+v", diagnostic)
				}
			}
			if tc.want == 1 {
				index := &ps6141SummaryIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
				found := false
				for fn := range index.declarations {
					if ps6090FunctionID(fn) == c.Consumer {
						found = true
						summary := index.function(fn)
						if !summary.valid || !summary.resultDeps[1] || !summary.resultDeps[2] || summary.traversals[1] != 1 || len(summary.writes) != 0 {
							t.Fatalf("receiver origin/effect proof missing: %+v", summary)
						}
					}
				}
				if !found {
					t.Fatal("typed method prerequisite absent")
				}
			}
		})
	}
}
