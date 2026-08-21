package ps6070

var ownerShapedCandidate = "#include <metal_stdlib>\nkernel void qmatmul_q4_0_cooperative(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) { int byteIdx=lane%16, hiNib=lane/16; for(int b=0;b<nb;b++){ int woff=woffRow+b*18; ushort pair=0; if(lane<8) pair=*((device const ushort*)(W+woff+2+lane*2)); pair=simd_shuffle(pair,byteIdx>>1); use(pair,hiNib); } }" // want `Metal packed-quant kernel qmatmul_q4_0_cooperative restricts 2-byte device loads to 8 of 32 lanes \(16-byte packed source span\) and executes 1 dynamic SIMD redistribution call\(s\) per block iteration`

var constantBroadcastSource = `#include <metal_stdlib>
kernel void qmatmul_q4_0_constant(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {
  for(int b=0;b<nb;b++) {
    ushort pair=0;
    if(lane<8) pair=*((device const ushort*)(W+b*18+lane*2));
    pair=simd_broadcast(pair,0);
  }
}`

var uniformLeaderAddress = `#include <metal_stdlib>
kernel void qmatmul_q4_0_uniform(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {
  for(int b=0;b<nb;b++) {
    ushort pair=0;
    if(lane<1) pair=*((device const ushort*)(W+b*18));
    pair=simd_shuffle(pair,lane>>1);
  }
}`

var commentedCandidate = `#include <metal_stdlib>
kernel void qmatmul_q4_0_comments(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {
  for(int b=0;b<nb;b++) {
    ushort pair=0;
    /* if(lane<8) pair=*((device const ushort*)(W+b*18+lane*2));
       pair=simd_shuffle(pair,lane>>1); */
    use(pair);
  }
}`
