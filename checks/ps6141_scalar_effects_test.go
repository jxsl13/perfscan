package checks

import (
	"os"
	"strings"
	"testing"
)

func TestPS6141AuthenticScalarHelperEffects(t *testing.T) {
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
		{"authenticHelpers", "", "", true},
		{"renamedHelpers", "f32ToF16", "sourceHalf", true},
		{"scalarWrapper", "return float32(math.Round(float64(v)))", "return leaf(v)};func leaf(v float32)float32{return float32(math.Round(float64(v)))", true},
		{"globalWrite", "b := math.Float32bits(f)", "global=f;b := math.Float32bits(f)", false},
		{"opaqueCall", "b := math.Float32bits(f)", "b := opaqueBits(f)", false},
		{"recursive", "b := math.Float32bits(f)", "b := uint32(f32ToF16(f))", false},
		{"panic", "b := math.Float32bits(f)", "panic(\"stop\");b := math.Float32bits(f)", false},
		{"conditionalPanic", "b := math.Float32bits(f)", "if f>0{panic(\"stop\")};b := math.Float32bits(f)", false},
		{"signedShiftUnknown", "drop := uint32(13) + uint32(-14-exp)", "drop := int32(13) + (-14-exp)", false},
		{"integerDivideUnknown", "b := math.Float32bits(f)", "b := math.Float32bits(f);b /= uint32(f)", false},
		{"storageCall", "b := math.Float32bits(f)", "opaqueStorage(bytes);b := math.Float32bits(f)", false},
		{"infiniteLoop", "b := math.Float32bits(f)", "for {};b := math.Float32bits(f)", false},
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
			source += "\nvar global float32;var bytes []byte;func opaqueStorage([]byte);func opaqueBits(float32)uint32\n"
			pass, _ := ps6141TypedFixture(t, source)
			found := 0
			for fn := range ps6099LocalFunctionDeclarations(pass) {
				if fn.Name() == "f32ToF16" || fn.Name() == "sourceHalf" || fn.Name() == "roundHalfAway" {
					found++
					summary := ps6141ScalarEffects(pass, fn)
					want := tc.want || fn.Name() == "roundHalfAway"
					if got := summary.effectsKnown && summary.completionKnown; got != want {
						t.Fatalf("%s summary=%+v want=%v", fn.Name(), summary, want)
					}
				}
			}
			if found != 2 {
				t.Fatal("authentic typed helper prerequisites missing")
			}
		})
	}
}
