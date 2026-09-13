package checks

import (
	"go/types"
	"os"
	"strings"
	"testing"
)

const ps6141PackedByteTraversalSynthetic = `package fixture
import "encoding/binary"
func dot(p,w []byte)float32{
 if len(p)!=len(w){panic("shape")};if len(p)%34!=0{panic("shape")}
 var sum float32
 for b:=range len(p)/34{
  pb:=p[b*34:b*34+34];wb:=w[b*34:b*34+34]
  dp:=float32(binary.LittleEndian.Uint16(pb));dw:=float32(binary.LittleEndian.Uint16(wb))
  for i,q:=range pb[2:]{sum+=float32(int8(q))*float32(int8(wb[2+i]))*dp*dw}
 };return sum
}
func owner(p,w []byte)float32{return dot(p,w)}`

func TestPS6141PackedByteTraversalLayout(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, new string
		want           bool
	}{
		{"syntheticGeometryPrerequisite", "", "", true},
		{"wrongBlock", "w[b*34:b*34+34]", "w[0:34]", false},
		{"wrongLane", "wb[2+i]", "wb[2+0*i]", false},
		{"mutatedCurrentLane", "sum+=", "i=0;sum+=", false},
		{"unknownTrip", "len(p)/34", "len(p)", false},
		{"incompleteExtent", "len(p)%34!=0", "len(p)%34==0", false},
		{"independentLaneOrigin", "range pb[2:]", "range wb[2:]", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141PackedByteTraversalSynthetic
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == ps6141PackedByteTraversalSynthetic {
					t.Fatal("mutation did not change fixture")
				}
			}
			pass, _ := ps6141TypedFixture(t, source)
			found := false
			for _, decl := range ps6099LocalFunctionDeclarations(pass) {
				if decl.Name.Name == "dot" {
					found = true
					proof := ps6141ByteConsumer(pass, decl)
					if got := proof != nil; got != tc.want {
						t.Fatalf("layout=%+v want=%v", proof, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("typed prerequisite missing")
			}
		})
	}
}

func TestPS6141PackedByteConsumerStorageEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, old, new string
		want           bool
	}{
		{"syntheticCompleteEffectPrerequisite", "", "", true},
		{"guardRootRebound", "var sum float32", "p=w;var sum float32", false},
		{"unreachableGuards", "if len(p)!=len(w)", "return 0;if len(p)!=len(w)", false},
		{"guardBypassed", "if len(p)!=len(w)", "goto Traverse;if len(p)!=len(w)", false},
		{"hiddenAlias", "var sum float32", "alias:=p;_ = alias;var sum float32", false},
		{"opaqueStorage", "var sum float32", "opaque(p);var sum float32", false},
		{"recoveredPanicStorage", "var sum float32", "if len(p)>0{panic(p)};var sum float32", false},
		{"hiddenFanout", "var sum float32", "for range 2{opaque(p)};var sum float32", false},
		{"viewWrite", "dp:=", "pb[0]=0;dp:=", false},
		{"viewEscape", "dp:=", "opaque(pb);dp:=", false},
		{"opaqueScalarEffect", "dp:=", "effect();dp:=", false},
		{"postTraversalRebind", "};return sum", "};p=w;return sum", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141PackedByteTraversalSynthetic
			if tc.old != "" {
				source = strings.ReplaceAll(source, tc.old, tc.new)
				if source == ps6141PackedByteTraversalSynthetic {
					t.Fatal("mutation did not change fixture")
				}
			}
			if tc.name == "guardBypassed" {
				source = strings.ReplaceAll(source, " var sum float32", "")
				source = strings.ReplaceAll(source, "goto Traverse;", "var sum float32;goto Traverse;")
				source = strings.ReplaceAll(source, "for b:=", "Traverse:for b:=")
			}
			source += "\nfunc opaque([]byte);func effect()\n"
			pass, _ := ps6141TypedFixture(t, source)
			found := false
			for _, decl := range ps6099LocalFunctionDeclarations(pass) {
				if decl.Name.Name == "dot" {
					found = true
					proof := ps6141ByteConsumer(pass, decl)
					got := proof != nil && proof.storageEffectsKnown && proof.completionKnown
					if got != tc.want {
						t.Fatalf("proof=%+v want=%v", proof, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("typed prerequisite missing")
			}
		})
	}
}

// Authentic source evidence for the scalar decode prerequisite only. The
// observed fused/native kernels are not activation-packing positives.
func TestPS6141AuthenticByteDecodePrerequisite(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/ps6141_authentic_q8_decode.go")
	if err != nil {
		t.Fatal(err)
	}
	base := string(raw)
	for _, tc := range []struct {
		name, old, new string
		want           bool
	}{
		{"authentic", "", "", true},
		{"renamed", "f16ToF32bits", "decodeHalf", true},
		{"missingNonzeroGuard", "if frac == 0", "if frac == 1", false},
		{"wrongRankingStep", "frac <<= 1", "frac >>= 1", false},
		{"unknownInitialExtent", "uint32(h) & 0x3FF", "uint32(h)", false},
		{"globalEffect", "sign := uint32(h>>15)", "f16Table[0]=1;sign := uint32(h>>15)", false},
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
			pass, _ := ps6141TypedFixture(t, source)
			found := false
			for fn := range ps6099LocalFunctionDeclarations(pass) {
				if fn.Name() == "f16ToF32bits" || fn.Name() == "decodeHalf" {
					found = true
					proof := ps6141ScalarEffects(pass, fn)
					if got := proof.effectsKnown && proof.completionKnown; got != tc.want {
						t.Fatalf("proof=%+v want=%v", proof, tc.want)
					}
					if tc.want {
						index := &ps6141SummaryIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
						flow := index.function(fn)
						if !flow.valid || !flow.resultDeps[1] || flow.resultSuppressed {
							t.Fatalf("authentic immutable returned-origin prerequisite=%+v", flow)
						}
					}
				}
				if fn.Name() == "f16ToF32" || fn.Name() == "dotQ8RowNeon" {
					proof := ps6141ScalarEffects(pass, fn)
					if proof.effectsKnown && proof.completionKnown {
						t.Fatalf("table/native source unexpectedly proven: %s", fn.Name())
					}
				}
			}
			if !found {
				t.Fatal("authentic decoder prerequisite missing")
			}
		})
	}
}
