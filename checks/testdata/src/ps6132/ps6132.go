package ps6132
type buffer interface{}
type bufSlot struct{ b buffer }
type block struct {gAttn,bAttn,gFFN,bFFN buffer}
type recorder interface {
 Blit(buffer,int,buffer,int,int)error
 LayerNorm(buffer,buffer,buffer,buffer,int,int,float32)error
 RMSNorm(buffer,buffer,buffer,int,int,float32)error
}
type Decoder struct {
 maxLen,d,qDim,kvDim,hidden,v,nExperts,logitsRows int
 postNorm,parallelTwoNorm,lnBias,moe bool
 eps float32
 invHost []float32
 ops struct{fusedGateUp,eagerFullLogits bool}
 dinv,dx,xn,xn2,q,k,v_,qkv,attn,gate,up,gu,logits,moeGate,moeW,moeCol *bufSlot
}
func firstErr(a,b error)error{if a!=nil{return a};return b}
func (d *Decoder) allocResidualScratch(mk func([]float32)*bufSlot,rows int){}
// Execution witnesses are verbatim statement selections from encodeStep
// (line3349) and stepN (lines3534/3605), not a claimed replay of either full method.
// Omitted execution/provider/recurrent paths are reviewed contract facts.
func (d *Decoder) encodeStep(r recorder,b block) error {
 e := d.recordAttnNorm(r, b, 1)
 return e
}
func (d *Decoder) stepN(r recorder,b block,tokens []int)error {
 k := len(tokens)
 e := d.recordAttnNorm(r, b, k)
 return e
}
// SPDX-License-Identifier: MPL-2.0
// Verbatim public GoAI functions from pre-change commit 74a7c5c923b25aa35773bb9e907b76aba04a553a.
// https://github.com/jxsl13/goai/blob/74a7c5c923b25aa35773bb9e907b76aba04a553a/llamagpu/decoder.go
// GoAI PR1210 merge ec20269a20e2028ec10aa02cd9d26095e8aa161b replaces the eager constructor.
func (d *Decoder) recordAttnNorm(r recorder, b block, rows int) error {
	if d.postNorm {
		return r.Blit(d.dx.b, 0, d.xn.b, 0, rows*d.d)
	}
	e := d.norm(r, d.dx.b, b.gAttn, b.bAttn, d.xn.b, rows)
	if d.parallelTwoNorm { // norm2(x0) BEFORE the attn add, for the FFN branch
		e = firstErr(e, d.norm(r, d.dx.b, b.gFFN, b.bFFN, d.xn2.b, rows))
	}
	return e
}
func (d *Decoder) norm(r recorder, x, gamma, beta, o buffer, rows int) error {
	if d.lnBias {
		return r.LayerNorm(x, gamma, beta, o, rows, d.d, d.eps)
	}
	return r.RMSNorm(x, gamma, o, rows, d.d, d.eps)
}
func (d *Decoder) allocScratch(mk func(data []float32) *bufSlot) {
	c := d.maxLen
	d.dinv = mk(d.invHost)
	d.dx = mk(make([]float32, c*d.d)) // want `constructor retains maximum-row transient`
	d.xn = mk(make([]float32, c*d.d)) // want `constructor retains maximum-row transient`
	d.xn2 = mk(make([]float32, c*d.d)) // want `constructor retains maximum-row transient`
	d.q = mk(make([]float32, c*d.qDim))
	d.k = mk(make([]float32, c*d.kvDim))
	d.v_ = mk(make([]float32, c*d.kvDim))
	d.qkv = mk(make([]float32, c*(d.qDim+2*d.kvDim)))
	d.attn = mk(make([]float32, c*d.qDim))
	d.allocResidualScratch(mk, c)
	d.gate = mk(make([]float32, c*d.hidden))
	d.up = mk(make([]float32, c*d.hidden))
	if d.ops.fusedGateUp {
		d.gu = mk(make([]float32, c*2*d.hidden)) // fused gate|up GEMV output for the SwiGLUHalves path
	}
	d.logitsRows = 1
	if d.ops.eagerFullLogits {
		d.logitsRows = c
	}
	d.logits = mk(make([]float32, d.logitsRows*d.v))
	if d.moe {
		d.moeGate = mk(make([]float32, c*d.nExperts))
		d.moeW = mk(make([]float32, c*d.nExperts))
		d.moeCol = mk(make([]float32, c))
	}
}

// Unreviewed lifecycle method is not a constructor candidate.
func (d *Decoder) ordinaryExecution(mk func([]float32)*bufSlot) { d.gate = mk(make([]float32,d.maxLen*d.hidden)) }
