package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6070(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6070.Analyzer, "ps6070")
}

func TestPS6070NativeSourceShapes(t *testing.T) {
	const ownerShape = `#include <metal_stdlib>
kernel void qmatmul_q4_0_cooperative(device const uchar* W [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  int byteIdx=lane%16, hiNib=lane/16;
  for (int b=0; b<nb; b++) {
    int woff=woffRow+b*18;
    ushort pair=0;
    if (lane < 8) pair=*((device const ushort*)(W+woff+2+lane*2));
    pair=simd_shuffle(pair, byteIdx>>1);
    use(pair, hiNib);
  }
}`
	tests := []struct {
		name       string
		source     string
		want       int
		loaders    int
		loadWidth  int
		sourceSpan int
	}{
		{name: "owner shape", source: ownerShape, want: 1, loaders: 8, loadWidth: 2, sourceSpan: 16},
		{
			name: "braced inclusive guard and indexed packed pointer",
			source: `#include <metal_stdlib>
kernel void dequant_packed(device const ushort* packed [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  for (int block=0; block<blocks; ++block) {
    ushort pair=0;
    if (lane <= 7) { pair=packed[block*8+lane]; }
    pair=simd_broadcast(pair, lane>>2);
  }
}`,
			want: 1, loaders: 8, loadWidth: 2, sourceSpan: 16,
		},
		{
			name: "objective c concatenated shader",
			source: `static NSString* shader =
"#include <metal_stdlib>\n"
"kernel void qmatmul_q4_0(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {\n"
" int byteIdx=lane%16; for(int b=0;b<nb;b++){ ushort pair=0;\n"
" if(lane<8){ pair=*((device const ushort*)(W+b*18+2+lane*2)); }\n"
" pair=simd_shuffle(pair, byteIdx>>1); use(pair); }\n"
"}\n";`,
			want: 1, loaders: 8, loadWidth: 2, sourceSpan: 16,
		},
		{
			name:   "constant source belongs to uniform broadcast review",
			source: strings.Replace(ownerShape, "byteIdx>>1", "0u", 1),
		},
		{
			name:   "single loader belongs to PS6068",
			source: strings.Replace(ownerShape, "lane < 8", "lane < 1", 1),
		},
		{
			name:   "all lanes load",
			source: strings.Replace(ownerShape, "lane < 8", "lane < 32", 1),
		},
		{
			name: "unguarded load",
			source: strings.Replace(ownerShape,
				"if (lane < 8) pair=*((device const ushort*)(W+woff+2+lane*2));",
				"pair=*((device const ushort*)(W+woff+2+lane*2));", 1),
		},
		{
			name:   "uniform address belongs to PS6068",
			source: strings.Replace(ownerShape, "W+woff+2+lane*2", "W+woff+2", 1),
		},
		{
			name: "byte load is not a packed load candidate",
			source: strings.Replace(ownerShape,
				"*((device const ushort*)(W+woff+2+lane*2))",
				"W[woff+2+lane]", 1),
		},
		{
			name: "redistribution outside block loop",
			source: `#include <metal_stdlib>
kernel void qmatmul_q4_0(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {
 ushort pair=0;
 if(lane<8) pair=*((device const ushort*)(W+lane*2));
 for(int b=0;b<nb;b++){ use(pair,b); }
 pair=simd_shuffle(pair,lane>>1);
}`,
		},
		{
			name:   "ordinary non quant kernel",
			source: strings.Replace(ownerShape, "qmatmul_q4_0_cooperative", "copy_words", 1),
		},
		{
			name: "validated annotation",
			source: strings.Replace(ownerShape, "kernel void",
				"//perfscan:packed-load-redistribution-validated same-binary crossover recorded.\nkernel void", 1),
		},
		{
			name: "commented candidate",
			source: `#include <metal_stdlib>
kernel void qmatmul_q4_0(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {
 for(int b=0;b<nb;b++){
  ushort pair=0;
  /* if(lane<8) pair=*((device const ushort*)(W+b*18+lane*2));
     pair=simd_shuffle(pair,lane>>1); */
  use(pair);
 }
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := ps6070PackedRedistributions(test.source)
			if len(findings) != test.want {
				t.Fatalf("findings = %#v, want %d", findings, test.want)
			}
			if test.want != 0 {
				finding := findings[0]
				if finding.loaders != test.loaders || finding.loadWidth != test.loadWidth || finding.sourceSpan != test.sourceSpan || finding.redistribute != 1 {
					t.Fatalf("finding = %#v, want loaders=%d width=%d span=%d redistributions=1", finding, test.loaders, test.loadWidth, test.sourceSpan)
				}
			}
		})
	}
}

func TestPS6070NumericSource(t *testing.T) {
	for _, source := range []string{"0", "0u", "(1UL)", "(ushort)0", "(0 + 1) << 2"} {
		if !ps6070NumericSource(source) {
			t.Errorf("%q should be a numeric source", source)
		}
	}
	for _, source := range []string{"byteIdx >> 1", "lane", "leader + 1"} {
		if ps6070NumericSource(source) {
			t.Errorf("%q should be a dynamic source", source)
		}
	}
}
