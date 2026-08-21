package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6069(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6069.Analyzer, "ps6069")
}

func TestPS6069Alignment(t *testing.T) {
	assignments := map[string]string{
		"rowBytes": "nsb*84",
		"rowOff":   "ni*rowBytes",
		"base":     "rowOff+sb*84",
		"qsB":      "base+16",
		"q0":       "qsB+nb*32",
		"gOff":     "grp*16",
		"lbase":    "hh*8",
	}
	if got := ps6069Alignment("q0+gOff+lbase", assignments, map[string]bool{}); got != 2 {
		t.Fatalf("Q2_K start alignment bits = %d, want 2 (four bytes)", got)
	}
	assignments["base"] = "rowOff+sb*88"
	assignments["rowBytes"] = "nsb*88"
	if got := ps6069Alignment("q0+gOff+lbase", assignments, map[string]bool{}); got != 3 {
		t.Fatalf("eight-aligned start bits = %d, want 3", got)
	}
}

func TestPS6069NativeSourceShapes(t *testing.T) {
	const ownerShape = `#include <metal_stdlib>
kernel void qmatmul_q2k_cooperative(device const float* X [[buffer(0)]],
    device const uchar* W [[buffer(1)]], ushort lane [[thread_index_in_simdgroup]]) {
  int nsb=K/256, rowBytes=nsb*84, rowOff=ni*rowBytes;
  short hh=lane%2, nb=lane/16, grp=(lane/2)%2;
  short gOff=grp*16, lbase=hh*8;
  for (int sb=0; sb<nsb; sb++) {
    int base=rowOff+sb*84, qsB=base+16, q0=qsB+nb*32;
    for (short l=lbase; l<lbase+8; l++) {
      int q2=(W[q0+gOff+l]>>2)&3;
      use(q2);
    }
  }
}`
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{name: "owner shape", source: ownerShape, want: 1},
		{
			name: "objective c shader storage",
			source: `static NSString* shader =
"#include <metal_stdlib>\n"
"kernel void qmatmul_q2k(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {\n"
" int rowBytes=nsb*84, rowOff=ni*rowBytes, gOff=grp*16, lbase=hh*8;\n"
" for(int sb=0;sb<nsb;sb++){ int base=rowOff+sb*84, q0=base+16+nb*32;\n"
"  for(short l=lbase;l<lbase+8;l++){ int q=W[q0+gOff+l]; use(q); } }\n"
"}\n";`,
			want: 1,
		},
		{
			name:   "eight byte aligned stride uses a different candidate",
			source: strings.ReplaceAll(ownerShape, "*84", "*88"),
		},
		{
			name:   "two byte aligned stride",
			source: strings.ReplaceAll(ownerShape, "*84", "*86"),
		},
		{
			name: "single loop",
			source: `#include <metal_stdlib>
kernel void qmatmul_q2k(device const uchar* W [[buffer(0)]]) {
 int base=block*84+16;
 for(short l=0;l<0+8;l++){ int q=W[base+l]; use(q); }
}`,
		},
		{
			name: "validated",
			source: `#include <metal_stdlib>
//perfscan:packed-quant-word-load-validated alignment and route campaigns recorded.
kernel void qmatmul_q2k(device const uchar* W [[buffer(0)]]) {
 for(int sb=0;sb<n;sb++){ int base=sb*84+16;
  for(short l=0;l<0+8;l++){ int q=W[base+l]; use(q); } }
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := ps6069PackedWordCandidates(test.source)
			if len(findings) != test.want {
				t.Fatalf("findings = %#v, want %d", findings, test.want)
			}
		})
	}
}
