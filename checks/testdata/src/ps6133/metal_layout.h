// SPDX-License-Identifier: MPL-2.0
// Frozen excerpt from jxsl13/GoAI backend/metal/metal_bridge.m
// parent 74a7c5c923b25aa35773bb9e907b76aba04a553a; retained verbatim, not native execution.
// rope2 (SPEC T613): ONE dispatch rotates BOTH bands of a fused QKV row in place — the q band
// (headsQ heads at element offset offQ) and the k band (headsK heads at offset offK), each row
// `stride` floats wide. Thread t covers pair i of virtual head h over seq rows: h < headsQ is a
// q-band head, the rest map to the k band. Replaces two back-to-back rope dispatches per layer.
// P={seq,stride,headsQ,offQ,headsK,offK,hd,half,posOffset}, F={posDiv}.
static NSString* const kRoPE2Source =
    @"#include <metal_stdlib>\n"
     "using namespace metal;\n"
     "kernel void rope2(device float* Q [[buffer(0)]],\n"
     "                  device const float* INV [[buffer(1)]],\n"
     "                  constant int* P [[buffer(2)]],\n"
     "                  constant float* F [[buffer(3)]],\n"
     "                  uint gid [[thread_position_in_grid]]) {\n"
     "  int seq=P[0], stride=P[1], headsQ=P[2], offQ=P[3], headsK=P[4], offK=P[5];\n"
     "  int hd=P[6], halfd=P[7], posOffset=P[8];\n"
     "  int hh=(headsQ+headsK)*halfd; int total=seq*hh;\n"
     "  if ((int)gid >= total) return;\n"
     "  int p=(int)gid/hh; int rem=(int)gid%hh; int h=rem/halfd; int i=rem%halfd;\n"
     "  int base = (h < headsQ) ? (offQ + p*stride + h*hd + i)\n"
     "                          : (offK + p*stride + (h-headsQ)*hd + i);\n"
     "  float pos=float(p+posOffset)/F[0];\n"
     "  float theta=INV[i];\n"
     "  float c=cos(pos*theta), s=sin(pos*theta);\n"
     "  float qi=Q[base], qih=Q[base+halfd];\n"
     "  Q[base]=qi*c-qih*s;\n"
     "  Q[base+halfd]=qih*c+qi*s;\n"
     "}\n"
     "kernel void rope2_split(device const float* QKV [[buffer(0)]],\n"
     "                        device const float* INV [[buffer(1)]],\n"
     "                        device float* QOut [[buffer(2)]],\n"
     "                        device float* KOut [[buffer(3)]],\n"
     "                        device float* VOut [[buffer(4)]],\n"
     "                        constant int* P [[buffer(5)]],\n"
     "                        constant float* F [[buffer(6)]],\n"
     "                        uint gid [[thread_position_in_grid]]) {\n"
     "  int seq=P[0], stride=P[1], headsQ=P[2], offQ=P[3], headsK=P[4], offK=P[5];\n"
     "  int hd=P[6], halfd=P[7], vOff=P[8], vDim=P[9], posOffset=P[10];\n"
     "  int pairCols=(headsQ+headsK)*halfd, rowWork=pairCols+vDim;\n"
     "  int total=seq*rowWork; if ((int)gid >= total) return;\n"
     "  int p=(int)gid/rowWork, rem=(int)gid%rowWork;\n"
     "  if (rem >= pairCols) { int j=rem-pairCols; VOut[p*vDim+j]=QKV[p*stride+vOff+j]; return; }\n"
     "  int h=rem/halfd, i=rem%halfd; bool isQ=h<headsQ;\n"
     "  int localH=isQ?h:(h-headsQ);\n"
     "  int src=(isQ?offQ:offK)+p*stride+localH*hd+i;\n"
     "  int dst=p*(isQ?headsQ*hd:headsK*hd)+localH*hd+i;\n"
     "  float pos=float(p+posOffset)/F[0], theta=INV[i];\n"
     "  float c=cos(pos*theta), s=sin(pos*theta);\n"
     "  float x=QKV[src], xh=QKV[src+halfd];\n"
     "  if (isQ) { QOut[dst]=x*c-xh*s; QOut[dst+halfd]=xh*c+x*s; }\n"
     "  else { KOut[dst]=x*c-xh*s; KOut[dst+halfd]=xh*c+x*s; }\n"
     "}\n";

static id<MTLComputePipelineState> gRoPE2 = nil;
static id<MTLComputePipelineState> gRoPE2Split = nil;

static int ensure_rope2(void) {
    if (gRoPE2 != nil && gRoPE2Split != nil) return 0;
    NSError* err = nil;
    id<MTLLibrary> lib = [gDevice newLibraryWithSource:kRoPE2Source options:nil error:&err];
    if (lib == nil) return -6;
    id<MTLFunction> fn = [lib newFunctionWithName:@"rope2"];
    id<MTLFunction> split = [lib newFunctionWithName:@"rope2_split"];
    if (fn == nil || split == nil) return -6;
    gRoPE2 = [gDevice newComputePipelineStateWithFunction:fn error:&err];
    gRoPE2Split = [gDevice newComputePipelineStateWithFunction:split error:&err];
    return (gRoPE2 != nil && gRoPE2Split != nil) ? 0 : -6;
}

// mtl_recorder_rope2 encodes the fused two-band in-place rotation (see kRoPE2Source) into the
// recorder's command buffer. No commit.
int mtl_recorder_rope2(void* rec, void* qh, void* invh,
                       int seq, int stride, int headsQ, int offQ, int headsK, int offK,
                       int hd, int half, int posOffset, float posDiv) {
    if (rec == NULL || qh == NULL || invh == NULL) return -2;
    if (ensure_rope2() != 0) return -6;
    id<MTLBuffer> qb = (__bridge id<MTLBuffer>)qh;
    id<MTLBuffer> ib = (__bridge id<MTLBuffer>)invh;
    int P[9] = {seq, stride, headsQ, offQ, headsK, offK, hd, half, posOffset};
    float F[1] = {posDiv};
    int total = seq * (headsQ + headsK) * half;
    id<MTLComputeCommandEncoder> enc = recorder_compute_encoder(rec, @"rope_pair");
    [enc setComputePipelineState:gRoPE2];
    [enc setBuffer:qb offset:0 atIndex:0];
    [enc setBuffer:ib offset:0 atIndex:1];
    [enc setBytes:P length:sizeof(P) atIndex:2];
    [enc setBytes:F length:sizeof(F) atIndex:3];
    NSUInteger tg = gRoPE2.maxTotalThreadsPerThreadgroup;
    if ((NSUInteger)total < tg) tg = (NSUInteger)total;
    [enc dispatchThreads:MTLSizeMake(total,1,1) threadsPerThreadgroup:MTLSizeMake(tg,1,1)];
    recorder_end_compute_encoder(rec, enc);
    return 0;
}

int mtl_recorder_rope2_split(
