package ps6069

var q2kCandidate = "#include <metal_stdlib>\nkernel void qmatmul_q2k_cooperative(device const float* X [[buffer(0)]], device const uchar* W [[buffer(1)]], ushort lane [[thread_index_in_simdgroup]]) { int nsb=K/256, rowBytes=nsb*84, rowOff=ni*rowBytes; short hh=lane%2, nb=lane/16, grp=(lane/2)%2; short gOff=grp*16, lbase=hh*8; for(int sb=0;sb<nsb;sb++){ int base=rowOff+sb*84, qsB=base+16, q0=qsB+nb*32; for(short l=lbase;l<lbase+8;l++){ int q2=(W[q0+gOff+l]>>2)&3; use(q2); } } }" // want `Metal packed-quant kernel qmatmul_q2k_cooperative reads an eight-byte W span in a nested byte loop whose full start offset`

var unaligned = `#include <metal_stdlib>
kernel void qmatmul_q2k_unaligned(device const uchar* W [[buffer(0)]]) {
  for (int sb=0; sb<n; sb++) { int base=sb*86+16;
    for (short l=0; l<0+8; l++) { int q=W[base+l]; use(q); }
  }
}`

var eightAligned = `#include <metal_stdlib>
kernel void qmatmul_q2k_eight(device const uchar* W [[buffer(0)]]) {
  for (int sb=0; sb<n; sb++) { int base=sb*88+16;
    for (short l=0; l<0+8; l++) { int q=W[base+l]; use(q); }
  }
}`
