package checks

import (
	"go/types"
	"os"
	"strings"
	"testing"
)

func TestPS6141AuthenticByteProducerComposition(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/ps6141_authentic_q8_0.go")
	if err != nil {
		t.Fatal(err)
	}
	base := string(raw)
	for _, tc := range []struct {
		name, old, new string
		want           bool
	}{
		{"authenticPrerequisite", "", "", true},
		{"renamedProducer", "quantizeQ8_0", "sourceCodec", true},
		{"scalarHelperWrapper", "f32ToF16(d)", "halfWrapper(d)", true},
		{"globalScalarWrite", "d := amax / 127", "global=amax;d := amax / 127", false},
		{"opaqueScalarEffect", "d := amax / 127", "d := opaqueScalar(amax)", false},
		{"helperGlobalEffect", "b := math.Float32bits(f)", "global=f;b := math.Float32bits(f)", false},
		{"helperRecursion", "b := math.Float32bits(f)", "b := uint32(f32ToF16(f))", false},
		{"terminalPanic", "d := amax / 127", "panic(\"stop\");d := amax / 127", false},
		{"conditionalPanic", "d := amax / 127", "if amax>0{panic(\"stop\")};d := amax / 127", false},
		{"lanePanic", "// bit-exact abs", "panic(\"lane\"); // bit-exact abs", false},
		{"unknownIntegerDivisor", "d := amax / 127", "q:=int(amax);q/=int(amax);d := amax / float32(q)", false},
		{"unboundedScalarLoop", "d := amax / 127", "for {};d := amax / 127", false},
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
			source += "\nvar global float32;func opaqueScalar(float32)float32;func halfWrapper(v float32)uint16{return f32ToF16(v)}\n"
			pass, _ := ps6141TypedFixture(t, source)
			declarations := ps6099LocalFunctionDeclarations(pass)
			found := false
			for fn := range declarations {
				if fn.Name() == "quantizeQ8_0" || fn.Name() == "sourceCodec" {
					found = true
					proof := ps6141ByteProducer(pass, fn)
					if !proof.layout.established {
						t.Fatal("independent authentic typed layout prerequisite failed")
					}
					if !proof.floatViewBoundsKnown {
						t.Fatal("affine float endpoint bound lost")
					}
					if got := proof.bodyEffectsKnown && proof.finiteBodyKnown; got != tc.want {
						t.Fatalf("bodyeffects=%v finite=%v want=%v", proof.bodyEffectsKnown, proof.finiteBodyKnown, tc.want)
					}
					if proof.boundsKnown || proof.layout.allocationArithmeticKnown || proof.layout.completionKnown {
						t.Fatal("unresolved runtime obligations incorrectly discharged")
					}
					index := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
					summary := index.producer(fn)
					if summary.valid || summary.byteProducer == nil {
						t.Fatal("partial composed producer was admitted or lost at boundary bridge")
					}
					if summary.byteProducer.bodyEffectsKnown != proof.bodyEffectsKnown || summary.byteProducer.finiteBodyKnown != proof.finiteBodyKnown {
						t.Fatal("boundary bridge lost composed body obligations")
					}
				}
			}
			if !found {
				t.Fatal("authentic typed producer missing")
			}
		})
	}
}
