package checks

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

const ps6132OwnerTypes = `package owner
type buffer interface{}
type bufSlot struct{ b buffer }
type block struct {gAttn,bAttn,gFFN,bFFN buffer}
type recorder interface {
 Blit(buffer,int,buffer,int,int)error
 LayerNorm(buffer,buffer,buffer,buffer,int,int,float32)error
 RMSNorm(buffer,buffer,buffer,int,int,float32)error
}
type Decoder struct {
 maxLen,d,qDim,kvDim,hidden,v,nExperts,logitsRows int
 postNorm,parallelTwoNorm,lnBias,moe bool
 eps float32
 invHost []float32
 ops struct{fusedGateUp,eagerFullLogits bool}
 dinv,dx,xn,xn2,q,k,v_,qkv,attn,gate,up,gu,logits,moeGate,moeW,moeCol *bufSlot
}
func firstErr(a,b error)error{if a!=nil{return a};return b}
func (d *Decoder) allocResidualScratch(mk func([]float32)*bufSlot,rows int){}
// Execution witnesses are verbatim statement selections from encodeStep
// (line3349) and stepN (lines3534/3605), not a claimed replay of either full method.
// Omitted execution/provider/recurrent paths are reviewed contract facts.
func (d *Decoder) encodeStep(r recorder,b block) error {
 e := d.recordAttnNorm(r, b, 1)
 return e
}
func (d *Decoder) stepN(r recorder,b block,tokens []int)error {
 k := len(tokens)
 e := d.recordAttnNorm(r, b, k)
 return e
}
`

func ps6132OwnerContract() config.ContextTransientWorkspaceContract {
	return config.ContextTransientWorkspaceContract{
		AllocationMethod: "owner.Decoder.allocScratch", AllocatorParameter: "mk", MaximumRowsField: "maxLen",
		ScratchFields: []string{"dx", "xn", "xn2"}, RowMethod: "owner.Decoder.recordAttnNorm", RowArgument: 2,
		GeometryPreservingMethods: []string{"owner.Decoder.allocResidualScratch"},
		OneRowExecutionMethod:     "owner.Decoder.encodeStep", BatchExecutionMethod: "owner.Decoder.stepN", BatchRowsBinding: "k",
		ConstructorOnlyAllocationReviewed: true, AllocatorCallbackOwnershipReviewed: true,
		AllExecutionPathsTransientActiveRowsReviewed: true, RecurrentOneRowPathsReviewed: true,
		ReceiverCallsSequential: true, CheckedGeometryReviewed: true, SynchronizationLifecycleReviewed: true, ExternalObservationReviewed: true,
	}
}

func TestPS6132PinnedOwnerConstructor(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6132_owner_constructor.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "0600ca23c3636c9f6d06de64fc2e96f8ee68d8762fb880efe191f38baf1149e5" {
		t.Fatalf("owner digest=%s", got)
	}
	for _, tc := range []struct {
		name, source string
		want         int
		change       func(*config.ContextTransientWorkspaceContract)
	}{
		{"unchanged_owner", ps6132OwnerTypes + string(data), 3, nil},
		{"owner_buffer_upload_receiver", strings.Replace(ps6132OwnerTypes, "type buffer interface{}", "type buffer interface{UploadF32([]float32)error}", 1) + string(data) + `func (d *Decoder) upload(host []float32)error{return d.dx.b.UploadF32(host)}`, 3, nil},
		{"captured_buffer_method", strings.Replace(ps6132OwnerTypes, "type buffer interface{}", "type buffer interface{UploadF32([]float32)error}", 1) + string(data) + `func (d *Decoder) upload(){fn:=d.dx.b.UploadF32;_=fn}`, 2, nil},
		{"resident_row", ps6132OwnerTypes + strings.ReplaceAll(string(data), "c*d.d", "1*d.d"), 0, nil},
		{"different_maximum", ps6132OwnerTypes + strings.Replace(string(data), "c := d.maxLen", "c := d.hidden", 1), 0, nil},
		{"ignored_helper_bound", ps6132OwnerTypes + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(data), "d.xn.b, rows)", "d.xn.b, 1)"), "d.xn2.b, rows)", "d.xn2.b, 1)"), "rows*d.d", "d.d"), 0, nil},
		{"rebound_helper_bound", ps6132OwnerTypes + strings.Replace(string(data), "if d.postNorm {", "rows,z:=d.maxLen,0;_=z;if d.postNorm {", 1), 0, nil},
		{"implicit_geometry_mutation", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "d.mutate();d.dx =", 1) + `func (d *Decoder) mutate(){d.maxLen++}`, 0, nil},
		{"captured_geometry_mutation", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "fn:=d.mutate;_=fn;d.dx =", 1) + `func (d *Decoder) mutate(){d.maxLen++}`, 0, nil},
		{"receiver_rebind", strings.Replace(ps6132OwnerTypes, "e := d.recordAttnNorm(r, b, 1)", "d,z:= &Decoder{},0;_=z;e := d.recordAttnNorm(r, b, 1)", 1) + string(data), 0, nil},
		{"mutated_snapshot", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "c++;d.dx =", 1), 0, nil},
		{"opaque_receiver_mutation", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "mutate(d);d.dx =", 1) + `func mutate(d *Decoder){d.maxLen++}`, 0, nil},
		{"receiver_alias_mutation", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "alias:=d;alias.maxLen++;d.dx =", 1), 0, nil},
		{"composite_receiver_escape", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "holder:=struct{D *Decoder}{d};holder.D.maxLen=1;d.dx =", 1), 0, nil},
		{"keyed_receiver_escape", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "holder:=struct{D *Decoder}{D:d};holder.D.maxLen=1;d.dx =", 1), 0, nil},
		{"external_whole_receiver_reset", ps6132OwnerTypes + string(data) + `func(d *Decoder)reset(){*d=Decoder{}}`, 0, nil},
		{"keyed_alternate_initializer", ps6132OwnerTypes + string(data) + `func alternate()*Decoder{return &Decoder{dx:&bufSlot{}}}`, 2, nil},
		{"allocator_rebind", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "existing:= &bufSlot{};mk=func([]float32)*bufSlot{return existing};d.dx =", 1), 0, nil},
		{"allocator_mixed_rebind", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "existing:= &bufSlot{};mk,z:=func([]float32)*bufSlot{return existing},0;_=z;d.dx =", 1), 0, nil},
		{"allocator_alias", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "alias:=mk;_=alias;d.dx =", 1), 0, nil},
		{"row_receiver_rebind", ps6132OwnerTypes + strings.Replace(string(data), "if d.postNorm {", "d,z:= &Decoder{},0;_=z;if d.postNorm {", 1), 0, nil},
		{"mixed_short_snapshot", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "c,z:=d.d,0;_=z;d.dx =", 1), 0, nil},
		{"range_snapshot", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "for c = range d.d {};d.dx =", 1), 0, nil},
		{"geometry_write", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "d.maxLen++;d.dx =", 1), 0, nil},
		{"whole_receiver_dereference", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "*d=Decoder{maxLen:1};d.dx =", 1), 0, nil},
		{"dereferenced_geometry", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "(*d).maxLen=1;d.dx =", 1), 0, nil},
		{"zero_helper_extent", ps6132OwnerTypes + strings.ReplaceAll(strings.ReplaceAll(string(data), ", rows)", ", rows*0)"), "rows*d.d", "rows*0"), 0, nil},
		{"addressed_snapshot", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "p:=&(c);_=p;d.dx =", 1), 0, nil},
		{"different_one_row", strings.Replace(ps6132OwnerTypes, "recordAttnNorm(r, b, 1)", "recordAttnNorm(r, b, 2)", 1) + string(data), 0, nil},
		{"different_batch_bound", strings.Replace(ps6132OwnerTypes, "recordAttnNorm(r, b, k)", "recordAttnNorm(r, b, k+1)", 1) + string(data), 0, nil},
		{"constant_batch", strings.Replace(ps6132OwnerTypes, "k := len(tokens)", "k := 1;_=tokens", 1) + string(data), 0, nil},
		{"batch_input_rebound", strings.Replace(ps6132OwnerTypes, "e := d.recordAttnNorm(r, b, k)", "tokens,z:=[]int{},0;_=z;_=tokens;e := d.recordAttnNorm(r, b, k)", 1) + string(data), 0, nil},
		{"unreachable_allocation", ps6132OwnerTypes + strings.Replace(string(data), "d.dx =", "return;d.dx =", 1), 0, nil},
		{"mixed_short_batch", strings.Replace(ps6132OwnerTypes, "e := d.recordAttnNorm(r, b, k)", "k,z:=1,0;_=z;e := d.recordAttnNorm(r, b, k)", 1) + string(data), 0, nil},
		{"range_batch", strings.Replace(ps6132OwnerTypes, "e := d.recordAttnNorm(r, b, k)", "for k = range 1 {};e := d.recordAttnNorm(r, b, k)", 1) + string(data), 0, nil},
		{"field_alias", ps6132OwnerTypes + string(data) + `func (d *Decoder) alias(){p:=d.dx;_=p}`, 2, nil},
		{"field_address", ps6132OwnerTypes + string(data) + `func (d *Decoder) alias(){p:= &(d.dx);_=p}`, 2, nil},
		{"alternate_initializer", ps6132OwnerTypes + string(data) + `func (d *Decoder) alias(){d.dx=&bufSlot{}}`, 2, nil},
		{"unreviewed_recurrent", ps6132OwnerTypes + string(data), 0, func(c *config.ContextTransientWorkspaceContract) { c.RecurrentOneRowPathsReviewed = false }},
		{"unreviewed_callback", ps6132OwnerTypes + string(data), 0, func(c *config.ContextTransientWorkspaceContract) { c.AllocatorCallbackOwnershipReviewed = false }},
		{"unreviewed_provider_paths", ps6132OwnerTypes + string(data), 0, func(c *config.ContextTransientWorkspaceContract) {
			c.AllExecutionPathsTransientActiveRowsReviewed = false
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ps6132OwnerContract()
			if tc.change != nil {
				tc.change(&c)
			}
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner/owner.go": tc.source})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			analyzer := *PS6132.Analyzer
			analyzer.Run = func(pass *analysis.Pass) (any, error) {
				n := 0
				original := pass.Report
				pass.Report = func(d analysis.Diagnostic) {
					n++
					if len(d.SuggestedFixes) != 0 {
						t.Error("unsafe suggested fix")
					}
				}
				result, err := runPS6132WithContracts(pass, []config.ContextTransientWorkspaceContract{c})
				pass.Report = original
				if n != tc.want {
					t.Errorf("findings=%d want%d", n, tc.want)
				}
				return result, err
			}
			analysistest.Run(t, dir, &analyzer, "owner")
		})
	}
}

func TestPS6132ContractIsolation(t *testing.T) {
	t.Parallel()
	c := ps6132OwnerContract()
	cfg := config.Config{ContextTransientWorkspaceContracts: []config.ContextTransientWorkspaceContract{c}}
	sets := cfg.Compile()
	cfg.ContextTransientWorkspaceContracts[0].ScratchFields[0] = "other"
	if sets.ContextTransientWorkspaceContracts[0].ScratchFields[0] != "dx" {
		t.Fatal("config aliases caller storage")
	}
	if config.UsableContextTransientWorkspaceContractCount([]config.ContextTransientWorkspaceContract{c, c}) != 0 {
		t.Fatal("duplicate owner accepted")
	}
	if !PS6132.NeedsConfig || PS6132.AutoFix {
		t.Fatal("unsafe metadata")
	}
}

func TestPS6132SourceFixture(t *testing.T) {
	t.Parallel()
	c := ps6132OwnerContract()
	c.AllocationMethod = strings.Replace(c.AllocationMethod, "owner.", "ps6132.", 1)
	c.RowMethod = strings.Replace(c.RowMethod, "owner.", "ps6132.", 1)
	c.OneRowExecutionMethod = strings.Replace(c.OneRowExecutionMethod, "owner.", "ps6132.", 1)
	c.BatchExecutionMethod = strings.Replace(c.BatchExecutionMethod, "owner.", "ps6132.", 1)
	c.GeometryPreservingMethods = []string{"ps6132.Decoder.allocResidualScratch"}
	analyzer := *PS6132.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6132WithContracts(pass, []config.ContextTransientWorkspaceContract{c})
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6132")
}
