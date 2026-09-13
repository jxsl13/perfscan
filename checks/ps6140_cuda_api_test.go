package checks

// ps6140CUDAFixtureAPI contains type-only external CUDA API metadata copied from
// github.com/jxsl13/goai commit 1802a1ab656d358938ba05ad36ca3fdedd28d81d.
// Section comments identify the pinned primary declaration files. Exported
// signatures are preserved; opaque private native fields are omitted. Bodiless
// declarations provide no implementation, native-effect, or execution evidence.
// The importer type-checks these APIs outside the authentic owner SSA package.
func ps6140CUDAFixtureAPI() string {
	return `package cuda

import ("unsafe"; "github.com/jxsl13/goai/tensor"; "github.com/jxsl13/goai/backend")

// backend/cuda/cuda.go
type ResidentB struct {  }
type ResidentVec struct {  }
type DeviceF32 struct {  }
func Available() bool
func MemInfo() (free, total uint64)
func NewResidentB(b *tensor.Tensor) (*ResidentB, error)
func (r *ResidentB) MatMul(a *tensor.Tensor) (*tensor.Tensor, error)
func (r *ResidentB) Embed(ids []int32) (*DeviceF32, error)
func (r *ResidentB) Free()
func NewResidentVec(v *tensor.Tensor) (*ResidentVec, error)
func (r *ResidentVec) Free()
func (d *DeviceF32) View(off, rows, cols int) (*DeviceF32, error)
func UploadF32(a *tensor.Tensor) (*DeviceF32, error)
func (r *ResidentB) MatMulDevice(a *DeviceF32) (*DeviceF32, error)
func (r *ResidentB) MatMulAccInto(a, c *DeviceF32) error
func (d *DeviceF32) ToHost() (*tensor.Tensor, error)
func (d *DeviceF32) GELU() error
func (d *DeviceF32) SiLU() error
func (d *DeviceF32) Add(other *DeviceF32) error
func (d *DeviceF32) Mul(other *DeviceF32) error
func (d *DeviceF32) SwiGLU(up *DeviceF32) error
func (d *DeviceF32) RMSNorm(gamma *ResidentVec, eps float32) error
func (d *DeviceF32) RMSNormTo(gamma *ResidentVec, eps float32) (*DeviceF32, error)
func (d *DeviceF32) Softmax() error
func (d *DeviceF32) CausalScaleMask(scale float32, offset int) error
func (d *DeviceF32) RoPE(attrs backend.RoPEAttrs) error
func (d *DeviceF32) MatMul(b *DeviceF32) (*DeviceF32, error)
func (d *DeviceF32) MatMulBT(b *DeviceF32) (*DeviceF32, error)
func MultiHeadAttention(q, k, v *DeviceF32, heads int, causal bool) (*DeviceF32, error)
func GroupedQueryAttention(q, k, v *DeviceF32, qHeads, kvHeads int, causal bool) (*DeviceF32, error)
func GroupedQueryAttentionKV(q, k, v *DeviceF32, qHeads, kvHeads int) (*DeviceF32, error)
func GroupedQueryAttentionTF32(q, k, v *DeviceF32, qHeads, kvHeads int, causal bool) (*DeviceF32, error)
func (d *DeviceF32) Rows() int
func (d *DeviceF32) Cols() int
func (d *DeviceF32) Clone() (*DeviceF32, error)
func (d *DeviceF32) Free()

// backend/cuda/recorder.go
type Recorder struct {  }
func NewRecorder() (*Recorder, error)
func (rec *Recorder) RMSNorm(x, g, o *DeviceF32, rows, dim int, eps float32) error
func (rec *Recorder) LayerNorm(x, g, b, o *DeviceF32, rows, dim int, eps float32) error
func (rec *Recorder) AddBias(x, b, o *DeviceF32, rows, n int) error
func (rec *Recorder) MatMul(a, b, c *DeviceF32, m, k, n int) error
func (rec *Recorder) MatMulAcc(a, b, c *DeviceF32, m, k, n int) error
func (rec *Recorder) RoPE(q, inv, o *DeviceF32, seq, width, heads, hd, half, pos int, posDiv float32) error
func (rec *Recorder) RoPEAt(q, inv, o *DeviceF32, off, seq, width, heads, hd, half, pos int, posDiv float32) error
func (rec *Recorder) RoPEPair(qkv, inv *DeviceF32, seq, stride, headsQ, offQ, headsK, offK, hd, half, pos int, posDiv float32) error
func (rec *Recorder) RoPEPartialPair(qkv, inv *DeviceF32, seq, stride, headsQ, offQ, headsK, offK, hd, rotaryDim, pos int, posDiv float32) error
func (rec *Recorder) Blit(src *DeviceF32, srcOff int, dst *DeviceF32, dstOff, n int) error
func (rec *Recorder) Copy2D(src *DeviceF32, srcOff, srcStride int, dst *DeviceF32, dstOff, dstStride, rows, rowFloats int) error
func (rec *Recorder) MHA(q, k, v, o *DeviceF32, sq, sk, dm, heads, kvHeads, dk, causal, window int, scale float32) error
func (rec *Recorder) MoEGate(logits, weights *DeviceF32, rows, e, k, raw int, scale float32) error
func (rec *Recorder) Conv1DStep(x, w, b, state, out *DeviceF32, d, k int) error
func (rec *Recorder) WKVStep(k, v, w, u, aa, bb, pp, out *DeviceF32, d int) error
func (rec *Recorder) SSMStep(u, delta, a, b, c, dskip, h, y *DeviceF32, d, n int) error
func (rec *Recorder) SSDStep(x, delta, a, b, c, dskip, state, y *DeviceF32, heads, headDim, groups, n int) error
func (rec *Recorder) RowAxpy(dst, src, arow *DeviceF32, rows, cols int) error
func (rec *Recorder) MHAALiBi(q, k, v, o, slopes *DeviceF32, sq, sk, dm, heads, kvHeads, dk, causal, window int, scale float32) error
func (rec *Recorder) MHABias(q, k, v, o, bias *DeviceF32, sq, sk, dm, heads, kvHeads, dk, causal, window int, scale float32) error
func (rec *Recorder) MHARect(q, k, v, o *DeviceF32, sq, sk, heads, kvHeads, dqk, dv, causal, window int, scale float32) error
func (rec *Recorder) MHACap(q, k, v, o *DeviceF32, sq, sk, dm, heads, kvHeads, dk, causal, window int, scale, cap float32) error
func (rec *Recorder) Unary(x, o *DeviceF32, op int) error
func (rec *Recorder) Binary(a, b, o *DeviceF32, op int) error
func (rec *Recorder) SwiGLUHalves(gu, out *DeviceF32, rows, hidden int) error
func (rec *Recorder) GeGLUHalves(gu, out *DeviceF32, rows, hidden int) error
func (rec *Recorder) QMatMulResident(x *DeviceF32, w *ResidentBQ8, o *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAcc(x *DeviceF32, w *ResidentBQ8, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentQ4K(x *DeviceF32, w *ResidentBQ4K, o *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAccQ4K(x *DeviceF32, w *ResidentBQ4K, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAccQ6K(x *DeviceF32, w *ResidentBQ6K, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAccQ5K(x *DeviceF32, w *ResidentBQ5K, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAccQ2K(x *DeviceF32, w *ResidentBQ2K, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentAccQ3K(x *DeviceF32, w *ResidentBQ3K, dst *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentQ6K(x *DeviceF32, w *ResidentBQ6K, o *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentQ5K(x *DeviceF32, w *ResidentBQ5K, o *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentQ2K(x *DeviceF32, w *ResidentBQ2K, o *DeviceF32, m int) error
func (rec *Recorder) QMatMulResidentQ3K(x *DeviceF32, w *ResidentBQ3K, o *DeviceF32, m int) error
func (rec *Recorder) Commit() error
func (rec *Recorder) Wait() error
func (rec *Recorder) Finish() error
func (rec *Recorder) Free()

// backend/cuda/cuda_paged_kv.go
type PagedKVPool struct {  }
type SeqKV struct {  }
type PagedBatchView struct {  }
func NewPagedKVPool(numBlocks, blockSize, wkv int) (*PagedKVPool, error)
func NewPagedKVPoolI8(numBlocks, blockSize, kvHeads, hd int) (*PagedKVPool, error)
func (p *PagedKVPool) FreeBlocks() int
func (p *PagedKVPool) NumBlocks() int
func (p *PagedKVPool) NewSeqSharingPrefix(base *SeqKV, sharedBlocks int) (*SeqKV, error)
func (p *PagedKVPool) NewSeqFromBlocks(blockIds []int32) *SeqKV
func (p *PagedKVPool) Free()
func (p *PagedKVPool) NewSeqKV() *SeqKV
func (s *SeqKV) Len() int
func (s *SeqKV) Advance(delta int)
func (s *SeqKV) Append(k, v *DeviceF32) error
func (s *SeqKV) Reserve1() error
func (p *PagedKVPool) AppendBatched(seqs []*SeqKV, dk, dv *DeviceF32, view *PagedBatchView) error
func (p *PagedKVPool) AppendBatchedDev(dk, dv *DeviceF32, view *PagedBatchView) error
func (s *SeqKV) GatherK() (*DeviceF32, error)
func (s *SeqKV) GatherV() (*DeviceF32, error)
func (s *SeqKV) BlockTable() []int32
func (s *SeqKV) Release()
func (p *PagedKVPool) UploadBatchView(seqs []*SeqKV) (*PagedBatchView, error)
func (v *PagedBatchView) Update(seqs []*SeqKV) error
func (v *PagedBatchView) UpdateLens(seqs []*SeqKV) error
func (v *PagedBatchView) BumpLens(delta int) error
func (v *PagedBatchView) Free()
func (p *PagedKVPool) BatchedDecodeAttnView(q *DeviceF32, view *PagedBatchView, qHeads, kvHeads int) (*DeviceF32, error)
func (p *PagedKVPool) BatchedDecodeAttnViewInto(q *DeviceF32, view *PagedBatchView, qHeads, kvHeads int, out *DeviceF32) error
func (p *PagedKVPool) BatchedDecodeAttnViewGQA(q *DeviceF32, view *PagedBatchView, qHeads, kvHeads int) (*DeviceF32, error)
func (p *PagedKVPool) BatchedDecodeAttnViewGQAQio(q *DeviceF16, view *PagedBatchView, qHeads, kvHeads int) (*DeviceF16, error)
func (p *PagedKVPool) BatchedDecodeAttnViewGQAf16(q *DeviceF32, view *PagedBatchView, qHeads, kvHeads int) (*DeviceF32, error)
func (p *PagedKVPool) BatchedDecodeAttnViewGQAf16Qio(q16 unsafe.Pointer, qCols int, view *PagedBatchView, qHeads, kvHeads int) (unsafe.Pointer, error)
func (p *PagedKVPool) BatchedDecodeAttnViewSK(q *DeviceF32, view *PagedBatchView, qHeads, kvHeads, splitK int) (*DeviceF32, error)
func (p *PagedKVPool) BatchedDecodeAttn(q *DeviceF32, seqs []*SeqKV, qHeads, kvHeads int) (*DeviceF32, error)

// backend/cuda/cuda_devicef16.go
type DeviceF16 struct {  }
func NewDeviceF16(rows, cols int) (*DeviceF16, error)
func (d *DeviceF16) Rows() int
func (d *DeviceF16) Cols() int
func (d *DeviceF16) Free()
func F16FromF32(src *DeviceF32) (*DeviceF16, error)
func (d *DeviceF16) ToF32() (*DeviceF32, error)
func (d *DeviceF16) RMSNormInto(gamma *ResidentVec, eps float32, out *DeviceF16) error
func (r *ResidentBF16) MatMulF16(a *DeviceF16) (*DeviceF16, error)
func (r *ResidentBF16) MatMulF16AddInto(a, c *DeviceF16) error
func (d *DeviceF16) RoPE(inv *DeviceF32, attrs backend.RoPEAttrs) error
func (d *DeviceF16) RoPERagged(inv *DeviceF32, positions []int32, attrs backend.RoPEAttrs) error
func (d *DeviceF16) SwiGLU(up *DeviceF16) error
func (d *DeviceF16) Add(src *DeviceF16) error
func (d *DeviceF16) BatchArgmax() ([]int32, error)
func (d *DeviceF16) RoPEDpos(inv *DeviceF32, attrs backend.RoPEAttrs, pos *DevicePos) error

// backend/cuda/cuda_bf16_weight.go
type DeviceBf16 struct {  }
func NewDeviceBf16(rows, cols int) (*DeviceBf16, error)
func (d *DeviceBf16) FromF32(src *DeviceF32) error
func (d *DeviceBf16) Free()
func MatMulWBf16(a *DeviceF32, w *DeviceBf16, out *DeviceF32) error
func MatMulGradXWBf16(dY *DeviceF32, w *DeviceBf16, dX *DeviceF32) error

// backend/cuda/cuda_dpos.go
type DevicePos struct {  }
func NewDevicePos() (*DevicePos, error)
func (p *DevicePos) Set(pos int) error
func (p *DevicePos) Free()
func (d *DeviceF32) RoPEDpos(attrs backend.RoPEAttrs, pos *DevicePos) error
func (d *DeviceF32) RoPERagged(inv *DeviceF32, positions []int32, attrs backend.RoPEAttrs) error
func BuildRoPEInv(hd int, base float64) (*DeviceF32, error)
func (d *DeviceF32) RoPEDposInv(heads int, inv *DeviceF32, pos *DevicePos, posScale float64) error
func (c *KVCache) AppendDpos(k, v *DeviceF32, pos *DevicePos) error
func GroupedQueryAttentionKVDpos(q, k, v *DeviceF32, qHeads, kvHeads int, off *DevicePos) (*DeviceF32, error)
func (p *DevicePos) Ptr() unsafe.Pointer

// backend/cuda/cuda_graph.go
type CapturedGraph struct {  }
func CaptureBegin() error
func CaptureEnd() (*CapturedGraph, error)
func (g *CapturedGraph) Launch() error
func GraphSync() error
func (g *CapturedGraph) Free()

// backend/cuda/cuda_kvcache.go
type KVCache struct {  }
type KVCacheF16 struct {  }
type KVCacheI8 struct {  }
func NewKVCache(maxSeq, wkv int) (*KVCache, error)
func (c *KVCache) Append(k, v *DeviceF32) error
func (c *KVCache) K() *DeviceF32
func (c *KVCache) V() *DeviceF32
func (c *KVCache) Len() int
func (c *KVCache) SetLen(n int)
func (c *KVCache) Free()
func NewKVCacheF16(maxSeq, wkv int) (*KVCacheF16, error)
func (c *KVCacheF16) AppendDpos(k, v *DeviceF32, pos *DevicePos) error
func (c *KVCacheF16) PrefillKV(k, v *DeviceF32, m int) error
func (c *KVCacheF16) ZeroCache() error
func (c *KVCacheF16) Free()
func GroupedQueryAttentionKVF16DposFlashInto(q *DeviceF32, c *KVCacheF16, qHeads, kvHeads int, off *DevicePos, out *DeviceF32) error
func NewKVCacheI8(maxSeq, kvHeads, hd int) (*KVCacheI8, error)
func (c *KVCacheI8) AppendDpos(k, v *DeviceF32, pos *DevicePos) error
func (c *KVCacheI8) Free()
func GroupedQueryAttentionKVI8DposFlashInto(q *DeviceF32, c *KVCacheI8, qHeads, kvHeads int, off *DevicePos, out *DeviceF32) error
func (c *KVCacheI8) PrefillKV(k, v *DeviceF32, m int) error

// backend/cuda/cuda_f16.go
type ResidentBF16 struct {  }
func NewResidentBF16(b *tensor.Tensor) (*ResidentBF16, error)
func (r *ResidentBF16) MatMulDevice(a *DeviceF32) (*DeviceF32, error)
func (r *ResidentBF16) MatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBF16) Free()

// backend/cuda/cuda_quant.go
type ResidentBQ8 struct {  }
func NewResidentBQ8(b *tensor.Tensor) (*ResidentBQ8, error)
func NewResidentBQ8FromBlocks(raw []byte, k, n int) (*ResidentBQ8, error)
func (r *ResidentBQ8) QMatMulDevice(a *DeviceF32) (*DeviceF32, error)
func (r *ResidentBQ8) QMatMulMoeInto(a, out, moeGate *DeviceF32, m, gateStride, expertIdx int) error
func (r *ResidentBQ8) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ8) QMatMulSwiGLUInto(a, gate, out *DeviceF32) error
func (r *ResidentBQ8) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ8) Free()
func (r *ResidentBQ8) Close() error

// backend/cuda/cuda_quant_q2k.go
type ResidentBQ2K struct {  }
func NewResidentBQ2KFromBlocks(raw []byte, k, n int) (*ResidentBQ2K, error)
func (r *ResidentBQ2K) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ2K) QMatMulWMMAInto(a, out *DeviceF32) error
func (r *ResidentBQ2K) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ2K) Free()
func (r *ResidentBQ2K) Close() error

// backend/cuda/cuda_quant_q3k.go
type ResidentBQ3K struct {  }
func NewResidentBQ3KFromBlocks(raw []byte, k, n int) (*ResidentBQ3K, error)
func (r *ResidentBQ3K) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ3K) QMatMulWMMAInto(a, out *DeviceF32) error
func (r *ResidentBQ3K) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ3K) Free()
func (r *ResidentBQ3K) Close() error

// backend/cuda/cuda_quant_q4k.go
type ResidentBQ4K struct {  }
func NewResidentBQ4KFromBlocks(raw []byte, k, n int) (*ResidentBQ4K, error)
func (r *ResidentBQ4K) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ4K) QMatMulMoeInto(a, out, moeGate *DeviceF32, m, gateStride, expertIdx int) error
func (r *ResidentBQ4K) QMatMulWMMAInto(a, out *DeviceF32) error
func (r *ResidentBQ4K) QMatMulSwiGLUInto(a, gate, out *DeviceF32) error
func (r *ResidentBQ4K) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ4K) QMatMulWMMAAccInto(a, c *DeviceF32, m int) error
func (r *ResidentBQ4K) Free()
func (r *ResidentBQ4K) Close() error

// backend/cuda/cuda_quant_q5k.go
type ResidentBQ5K struct {  }
func NewResidentBQ5KFromBlocks(raw []byte, k, n int) (*ResidentBQ5K, error)
func (r *ResidentBQ5K) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ5K) QMatMulWMMAInto(a, out *DeviceF32) error
func (r *ResidentBQ5K) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ5K) Free()
func (r *ResidentBQ5K) Close() error

// backend/cuda/cuda_quant_q6k.go
type ResidentBQ6K struct {  }
func NewResidentBQ6KFromBlocks(raw []byte, k, n int) (*ResidentBQ6K, error)
func (r *ResidentBQ6K) QMatMulInto(a, out *DeviceF32) error
func (r *ResidentBQ6K) QMatMulWMMAInto(a, out *DeviceF32) error
func (r *ResidentBQ6K) QMatMulAccInto(a, c *DeviceF32) error
func (r *ResidentBQ6K) Free()
func (r *ResidentBQ6K) Close() error

// backend/cuda/cuda_into.go
func NewDeviceF32(rows, cols int) (*DeviceF32, error)
func (d *DeviceF32) Argmax() int
func (d *DeviceF32) SoftmaxStatsN(n int, temperature float64) (maxLogit, zexp float64, err error)
func (d *DeviceF32) TopK(k int) ([]int32, []float32, error)
func (d *DeviceF32) TopKN(n, k int) ([]int32, []float32, error)
func (d *DeviceF32) LayerNormInto(gamma, beta *ResidentVec, eps float32, out *DeviceF32) error
func (d *DeviceF32) AddBias(bias *ResidentVec) error
func (d *DeviceF32) CopyFrom(src *DeviceF32) error
func (d *DeviceF32) Blit(dstOff int, src *DeviceF32, srcOff, n int) error
func (d *DeviceF32) Copy2D(dstOff, dstStride int, src *DeviceF32, srcOff, srcStride, rows, rowFloats int) error
func (d *DeviceF32) RoPEAtBand(inv *DeviceF32, off, heads, hd, posOffset int, posDiv float64, stride int) error
func (d *DeviceF32) RoPEPairBand(inv *DeviceF32, stride, headsQ, offQ, headsK, offK, hd, posOffset int, posDiv float64) error
func NewDeviceBufferF32(data []float32) (*DeviceF32, error)
func (d *DeviceF32) UploadF32(data []float32) error
func (d *DeviceF32) DownloadF32(dst []float32) error
func (d *DeviceF32) Release()
func (d *DeviceF32) Zero() error
func (r *ResidentB) MatMulInto(a, out *DeviceF32) error
func (d *DeviceF32) RMSNormInto(gamma *ResidentVec, eps float32, out *DeviceF32) error
func (r *ResidentB) EmbedInto(ids []int32, out *DeviceF32) error
func GroupedQueryAttentionKVDposInto(q, k, v *DeviceF32, qHeads, kvHeads int, off *DevicePos, scores, out *DeviceF32) error
func GroupedQueryAttentionKVDposFlashInto(q, k, v *DeviceF32, qHeads, kvHeads int, off *DevicePos, out *DeviceF32) error
func (c *KVCache) FullView() (kk, vv *DeviceF32)
func (c *KVCache) ZeroCache() error`
}
