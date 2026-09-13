package checks

import (
	"github.com/jxsl13/perfscan/config"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"os"
	"strings"
	"testing"
)

// MIXED source composition: authentic weight encoder and reference scalar half
// decoder, with a controlled packed-activation owner/dot. Not observed activation
// provenance, a native equivalence oracle, or performance evidence.
func TestPS6141MixedPackedByteProtocolJoin(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/ps6141_authentic_q8_0.go")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := os.ReadFile("testdata/ps6141_authentic_q8_decode.go")
	if err != nil {
		t.Fatal(err)
	}
	decode := string(decoded)
	start := strings.Index(decode, "func f16ToF32bits(")
	end := strings.Index(decode, "func scalarQ8RowReference(")
	if start < 0 || end <= start {
		t.Fatal("authentic decoder prerequisite missing")
	}
	decode = decode[start:end]
	dot := ps6141PackedByteTraversalSynthetic
	start = strings.Index(dot, "func dot(")
	end = strings.Index(dot, "func owner(")
	dot = dot[start:end]
	dot = strings.ReplaceAll(dot, "float32(binary.LittleEndian.Uint16(", "f16ToF32bits(binary.LittleEndian.Uint16(")
	harness := `func owner(x []float32,w []byte)float32{if len(x)/32>1024{panic("shape")};p:=quantizeQ8_0(x);return dot(p,w)}` + "\n" + dot + "\n" + decode
	base := strings.Replace(string(raw), "func owner(x []float32) []byte { return quantizeQ8_0(x) }", harness, 1)
	if base == string(raw) {
		t.Fatal("owner scaffold not replaced")
	}
	for _, tc := range []struct {
		name, old, new string
		want           int
	}{
		{"genuineMixedPrerequisite", "", "", 1},
		{"wrapper", "func dot(p,w []byte)float32{", "func dot(p,w []byte)float32{return leaf(p,w)};func leaf(p,w []byte)float32{", 1},
		{"wrapperPermuted", "func dot(p,w []byte)float32{", "func dot(p,w []byte)float32{return leaf(w,p)};func leaf(p,w []byte)float32{", 1},
		{"wrapperConflictingOrigins", "func dot(p,w []byte)float32{", "func dot(p,w []byte)float32{return leaf(p,p)};func leaf(p,w []byte)float32{", 0},
		{"unsignedLane", "int8(q)", "uint8(q)", 0},
		{"unsignedRoundTrip", "int8(q)", "uint8(int8(q))", 0},
		{"maskedSign", "int8(q)", "(int8(q)&0x7f)", 0},
		{"helperUnsignedRoundTrip", "int8(q)", "unsignedLane(int8(q))", 0},
		{"helperMaskedSign", "int8(q)", "maskedLane(int8(q))", 0},
		{"wrappedHelperMask", "int8(q)", "wrapMasked(int8(q))", 0},
		{"helperCompoundMask", "int8(q)", "compoundMask(int8(q))", 0},
		{"exactCheckedShape", "len(x)/32>1024", "len(x)!=32", 1},
		{"exactCheckedPolicy", "len(x)/32>1024", "len(x)!=32", 0},
		{"wrongCheckedPolicyShape", "len(x)/32>1024", "len(x)!=32", 1},
		{"incompleteCheckedShape", "len(x)/32>1024", "len(x)!=33", 0},
		{"missingHeaderContribution", "*dp*dw", "+0*dp+0*dw", 0},
		{"ignoredDecodeResult", "dp:=f16ToF32bits(binary.LittleEndian.Uint16(pb))", "dp:=ignored(binary.LittleEndian.Uint16(pb))", 0},
		{"wrongStride", "34", "35", 0},
		{"laneOverwrite", "sum+=", "sum=", 0},
		{"laneReset", "sum+=", "sum=0;sum+=", 0},
		{"ignoredReturnedResult", "return sum", "return float32(int(sum)&0)", 0},
		{"NOutputReuse", "return dot(p,w)", "var result float32;for range 2{result+=dot(p,w)};return result", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := base
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == base {
					t.Fatal("mutation did not change exact source")
				}
			}
			source += "\nfunc ignored(h uint16)float32{return 0};func unsignedLane(q int8)uint8{return uint8(q)};func maskedLane(q int8)int8{return q&0x7f};func wrapMasked(q int8)int8{return maskedLane(q)};func compoundMask(q int8)int8{v:=q;v &=0x7f;return v}\n"
			pass, owner := ps6141TypedFixture(t, source)
			c := ps6141TestContract()
			c.Quantizer = "fixture.quantizeQ8_0"
			c.Consumer = "fixture.dot"
			c.ConsumerForm = "packedByteDot"
			c.WeightArgument = 1
			c.RowsArgument = -1
			if tc.name == "wrapperPermuted" {
				source = strings.ReplaceAll(source, "return dot(p,w)", "return dot(w,p)")
				pass, owner = ps6141TypedFixture(t, source)
				c.PackedArgument = 1
				c.WeightArgument = 0
			}
			if !c.Valid() {
				t.Fatal("protocol contract invalid")
			}
			if tc.name == "exactCheckedPolicy" || tc.name == "wrongCheckedPolicyShape" {
				elements := int64(32)
				if tc.name == "wrongCheckedPolicyShape" {
					elements = 64
				}
				ps6141TestAttachSourcePolicy(t, pass, owner, source, &c, elements)
			}
			if got := len(ps6141Candidates(pass, owner, &c)); got != tc.want {
				index := &ps6141SummaryIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
				for fn, decl := range index.declarations {
					if fn.Name() == "dot" || fn.Name() == "leaf" {
						t.Logf("%s layout=%+v summary=%+v", fn.Name(), ps6141ByteConsumer(pass, decl), index.function(fn))
					}
				}
				t.Fatalf("candidates=%d want=%d", got, tc.want)
			}
			var diagnostics []analysis.Diagnostic
			pass.Report = func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }
			if _, err := runPS6141WithContracts(pass, []config.SingleUseQuantizationContract{c}); err != nil {
				t.Fatal(err)
			}
			if len(diagnostics) != tc.want {
				t.Fatalf("integrated advisory count=%d want=%d", len(diagnostics), tc.want)
			}
		})
	}
}
