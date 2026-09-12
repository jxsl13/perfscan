package checks

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func ps6129OwnerContract() config.CausalZeroGEMMContract {
	return config.CausalZeroGEMMContract{
		OwnerSite: "owner.mhaBwdGemmBand", GEMMFunction: "owner.gemmF32Rows", BoundsMethod: "owner.mhaGeo.bounds", ProbabilityHelper: "owner.mhaSoftmaxBandF32", ScratchAllocator: "owner.getF32Raw", ScratchRelease: "owner.putF32",
		NonRetainingBufferHelpers: []string{"owner.gemmF32RowsCols"},
		RowsBinding:               "iN", SequenceBinding: "seq", QueryOffsetBinding: "i0", GeometryBinding: "geo", SequenceField: "sq", CausalField: "causal", OffsetField: "off",
		Matrices:                []config.CausalZeroGEMMMatrix{{SourceBinding: "sb", TransposeBinding: "pt", Probability: true}, {SourceBinding: "da", TransposeBinding: "dat"}},
		BoundsSemanticsReviewed: true, SelfAttentionGeometryReviewed: true, ScratchOwnershipReviewed: true, HelperSemanticsReviewed: true, ProbabilityAllDispatchPathsClearReviewed: true,
	}
}

const ps6129OwnerStubs = `package owner
type mhaGeo struct {sq,sk,dk,dm,rep,off,window int;causal bool;scale float64}
func (g mhaGeo) bounds(i int)(int,int){jmax:=g.sk;if g.causal{jmax=g.off+i+1};jmin:=0;if g.window>0{if lo:=g.off+i-g.window+1;lo>0{jmin=lo}};return jmin,jmax}
func getF32Raw(n int)*[]float32{b:=make([]float32,n);return &b}
func putF32(b *[]float32){}
func mhaSoftmaxBandF32(b []float32,g mhaGeo,h,i0,iN int){}
func gemmF32Rows(a,b,c []float32,start,end,depth,columns int){}
func gemmF32RowsCols(a,b,c []float32,start,end,depth,columns,cstart,cend int){}
func escape(b []float32){}
func returnAlias(b []float32)[]float32{return b}
func mutateWholeBuffer(b []float32){b[0]=1}
func (g *mhaGeo) mutateOffset(){g.off++}
func (g *mhaGeo) mutateCausal(){g.causal=!g.causal}
func (g *mhaGeo) mutateSequence(){g.sq++}
`

func TestPS6129PinnedOwnerReplay(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/ps6129_owner_band.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Pin the public excerpt separately from the executable type stubs.
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != "d7c162a912e676584edb9fded29ef6579feb6e7aa0f6fc0605aaa8f3f216efef" {
		t.Fatalf("owner excerpt digest=%s", got)
	}
	for _, tc := range []struct {
		name, source string
		want         int
		change       func(*config.CausalZeroGEMMContract)
	}{
		{"unchanged_owner", string(data), 3, nil},
		{"altered_query_bound", strings.Replace(string(data), "geo.bounds(i0 + r)", "geo.bounds(r)", 1), 1, nil},
		{"partial_clear", strings.Replace(string(data), "clear(dar[jmax:])", "clear(dar[jmax:seq-1])", 1), 1, nil},
		{"missing_clear", strings.Replace(string(data), "clear(dar[jmax:])", "", 1), 1, nil},
		{"mutated_gradient", strings.Replace(string(data), "// dQ band", "da[seq-1]=1\n// dQ band", 1), 1, nil},
		{"mutated_probability", strings.Replace(string(data), "// dQ band", "sb[seq-1]=1\n// dQ band", 1), 2, nil},
		{"probability_mutated_inside_clear_loop", strings.Replace(string(data), "clear(dar[jmax:])", "pr[seq-1]=1; clear(dar[jmax:])", 1), 2, nil},
		{"post_transpose_mutation", strings.Replace(string(data), "gemmF32Rows(pt, gb, partV", "dat[seq*iN-1]=1; gemmF32Rows(pt, gb, partV", 1), 2, nil},
		{"early_pool_release", strings.Replace(string(data), "defer putF32(daP)", "putF32(daP)", 1), 0, nil},
		{"geometry_field_write", strings.Replace(string(data), "// dQ band", "geo.off++\n// dQ band", 1), 0, nil},
		{"geometry_field_address", strings.Replace(string(data), "// dQ band", "pointer:= &geo.off; _=pointer\n// dQ band", 1), 0, nil},
		{"pointer_receiver_offset", strings.Replace(string(data), "// dQ band", "geo.mutateOffset()\n// dQ band", 1), 0, nil},
		{"pointer_receiver_causal", strings.Replace(string(data), "// dQ band", "geo.mutateCausal()\n// dQ band", 1), 0, nil},
		{"pointer_receiver_sequence", strings.Replace(string(data), "// dQ band", "geo.mutateSequence()\n// dQ band", 1), 0, nil},
		{"captured_pointer_method", strings.Replace(string(data), "// dQ band", "fn:=geo.mutateOffset; _=fn\n// dQ band", 1), 0, nil},
		{"mixed_short_declaration_bound", strings.Replace(string(data), "clear(dar[jmax:])", "jmax,scratch:=seq,0;_=scratch;clear(dar[jmax:])", 1), 1, nil},
		{"range_assignment_bound", strings.Replace(string(data), "clear(dar[jmax:])", "for jmax = range seq {};clear(dar[jmax:])", 1), 1, nil},
		{"range_assignment_sequence", strings.Replace(string(data), "// dQ band", "for seq = range 1 {}\n// dQ band", 1), 0, nil},
		{"cross_row_store_before_clear", strings.Replace(string(data), "clear(dar[jmax:])", "da[seq-1]=1;clear(dar[jmax:])", 1), 1, nil},
		{"whole_matrix_helper_inside_clear_loop", strings.Replace(string(data), "clear(dar[jmax:])", "mutateWholeBuffer(da);clear(dar[jmax:])", 1), 1, func(c *config.CausalZeroGEMMContract) {
			c.NonRetainingBufferHelpers = append(c.NonRetainingBufferHelpers, "owner.mutateWholeBuffer")
		}},
		{"row_helper_inside_clear_loop", strings.Replace(string(data), "clear(dar[jmax:])", "mutateWholeBuffer(dar);clear(dar[jmax:])", 1), 1, func(c *config.CausalZeroGEMMContract) {
			c.NonRetainingBufferHelpers = append(c.NonRetainingBufferHelpers, "owner.mutateWholeBuffer")
		}},
		{"partial_row_clear", strings.Replace(string(data), "clear(dar[jmax:])", "if r%2==0{clear(dar[jmax:])}", 1), 1, nil},
		{"partial_transpose", strings.Replace(string(data), "for j := range seq {", "for j := range seq-1 {", 1), 1, nil},
		{"aliased_partitions", strings.Replace(string(data), "(*ptP)[seq*iN:]", "(*ptP)[:seq*iN]", 1), 0, nil},
		{"escaped_gradient", strings.Replace(string(data), "// dQ band", "escape(da)\n// dQ band", 1), 0, nil},
		{"configured_return_alias", strings.Replace(string(data), "// dQ band", "alias:=returnAlias(da);alias[seq-1]=1\n// dQ band", 1), 0, func(c *config.CausalZeroGEMMContract) {
			c.NonRetainingBufferHelpers = append(c.NonRetainingBufferHelpers, "owner.returnAlias")
		}},
		{"unreviewed_dispatch", string(data), 0, func(c *config.CausalZeroGEMMContract) { c.ProbabilityAllDispatchPathsClearReviewed = false }},
		{"unreviewed_pool", string(data), 0, func(c *config.CausalZeroGEMMContract) { c.ScratchOwnershipReviewed = false }},
		{"trimmed_consumers", strings.ReplaceAll(strings.ReplaceAll(string(data), "0, iN, seq, dk)", "0, iN, i0+iN, dk)"), "0, seq, iN, dk)", "0, i0+iN, iN, dk)"), 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner/owner.go": ps6129OwnerStubs + tc.source})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			c := ps6129OwnerContract()
			if tc.change != nil {
				tc.change(&c)
			}
			analyzer := &analysis.Analyzer{Name: "PS6129", Doc: "owner replay", Run: func(pass *analysis.Pass) (any, error) {
				count := 0
				clone := *pass
				clone.Report = func(d analysis.Diagnostic) {
					count++
					if len(d.SuggestedFixes) != 0 {
						t.Error("advisory has autofix")
					}
				}
				result, err := runPS6129WithContracts(&clone, []config.CausalZeroGEMMContract{c})
				if count != tc.want {
					t.Errorf("findings=%d want %d", count, tc.want)
				}
				return result, err
			}}
			analysistest.Run(t, dir, analyzer, "owner")
		})
	}
}

func TestPS6129CausalContractValidation(t *testing.T) {
	t.Parallel()
	base := ps6129OwnerContract()
	if !base.Valid() {
		t.Fatal("complete owner contract invalid")
	}
	for _, change := range []func(*config.CausalZeroGEMMContract){
		func(c *config.CausalZeroGEMMContract) { c.BoundsMethod = "invalid" },
		func(c *config.CausalZeroGEMMContract) {
			c.Matrices[1].TransposeBinding = c.Matrices[0].TransposeBinding
		},
		func(c *config.CausalZeroGEMMContract) { c.OffsetField = c.SequenceField },
		func(c *config.CausalZeroGEMMContract) { c.BoundsSemanticsReviewed = false },
		func(c *config.CausalZeroGEMMContract) { c.HelperSemanticsReviewed = false },
		func(c *config.CausalZeroGEMMContract) { c.ScratchOwnershipReviewed = false },
		func(c *config.CausalZeroGEMMContract) { c.ProbabilityAllDispatchPathsClearReviewed = false },
	} {
		c := ps6129OwnerContract()
		change(&c)
		if c.Valid() {
			t.Errorf("incomplete/contradictory contract valid: %+v", c)
		}
	}
	original := config.Config{CausalZeroGEMMContracts: []config.CausalZeroGEMMContract{ps6129OwnerContract()}}
	compiled := original.Compile()
	original.CausalZeroGEMMContracts[0].Matrices[0].SourceBinding = "changed"
	original.CausalZeroGEMMContracts[0].NonRetainingBufferHelpers[0] = "changed"
	if compiled.CausalZeroGEMMContracts[0].Matrices[0].SourceBinding != "sb" || compiled.CausalZeroGEMMContracts[0].NonRetainingBufferHelpers[0] != "owner.gemmF32RowsCols" {
		t.Fatal("compiled contract aliases mutable input")
	}
	if config.UsableCausalZeroGEMMContractCount([]config.CausalZeroGEMMContract{base, base}) != 0 {
		t.Fatal("duplicate owner claims usable")
	}
}
