package checks

import (
	"os"
	"strings"
	"testing"
)

func TestPS6141AuthenticByteBlockLayoutControls(t *testing.T) {
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
		{"renamedObjectsAndCodec", "quantizeQ8_0", "sourceCodec", true},
		{"wrongBlock", "x[b*blockElems : (b+1)*blockElems]", "x[0 : blockElems]", false},
		{"wrongLane", "out[o+2+j]", "_ = j;out[o+2]", false},
		{"wrongStride", "nb*34", "nb*64", false},
		{"wrongHeaderOffset", "PutUint16(out[o:]", "PutUint16(out[0:]", false},
		{"indexMutation", "for j, v := range blk {", "for j, v := range blk { j=0;", false},
		{"blockMutation", "blk := x[b*blockElems", "b=0;blk := x[b*blockElems", false},
		{"inputMutation", "var amax float32", "blk[0]=0;var amax float32", false},
		{"storageEscape", "return out", "cache=out;return out", false},
		{"viewEscape", "var amax float32", "floatCache=blk;var amax float32", false},
		{"opaqueStorage", "var amax float32", "opaqueBytes(out);var amax float32", false},
		{"capturedLane", "out[o+2+j] =", "capture:=func()int{return j};_ = capture;out[o+2+j] =", false},
		{"unknownTrip", "for b := range nb", "for b := range opaqueTrip()", false},
		{"nestedLaneFanout", "out[o+2+j] = byte(int8(roundHalfAway(v * id)))", "for row:=range 2{_ = row;out[o+2+j] = byte(int8(roundHalfAway(v * id)))}", false},
		{"reusedScratch", "out := make([]byte, nb*34)", "out := cache", false},
		{"duplicateLaneWrites", "out[o+2+j] = byte(int8(roundHalfAway(v * id)))", "out[o+2+j] = byte(int8(roundHalfAway(v * id)));out[o+2+j] = 0", false},
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
			source += "\nvar cache []byte;var floatCache []float32;func opaqueBytes([]byte);func opaqueTrip() int\n"
			pass, _ := ps6141TypedFixture(t, source)
			declarations := ps6099LocalFunctionDeclarations(pass)
			found := false
			for fn, decl := range declarations {
				if fn.Name() == "quantizeQ8_0" || fn.Name() == "sourceCodec" {
					found = true
					if got := ps6141ByteBlocks(pass, decl).established; got != tc.want {
						t.Fatalf("layout=%v want=%v", got, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("typed source producer missing")
			}
		})
	}
}
