package ps6133

/*
#include <stdint.h>
static int mtl_recorder_rope2(void* rec,void* q,void* inv,int seq,int stride,int hq,int oq,int hk,int ok,int hd,int half,int pos,float div){return 0;}
static int vk_recorder_rope2(void* rec,const uint32_t* spv,int len,void* q,void* inv,int seq,int stride,int hq,int oq,int hk,int ok,int hd,int half,int pos,float div){return 0;}
*/
import "C"
import "fmt"
import "unsafe"

type DeviceBuffer struct {
	n      int
	handle unsafe.Pointer
}
type Recorder struct{ handle unsafe.Pointer }

var rope2Spirv []byte

// Native stub is cgo ABI/type scaffolding only; no native execution.
func (r *Recorder) RoPEPair(qkv, inv *DeviceBuffer, seq, stride, headsQ, offQ, headsK, offK, hd, half, posOffset int, posDiv float32) error {
	maxOff := max(offQ, offK)
	if offQ < 0 || offK < 0 || qkv.n < maxOff+seq*stride || inv.n < half { // want `conditional on positive representable geometry`
		return fmt.Errorf("metal: Recorder rope2 shape mismatch: qkv=%d (want %d at off %d) inv=%d (want %d)", qkv.n, seq*stride, maxOff, inv.n, half)
	}
	rc := C.mtl_recorder_rope2(r.handle, qkv.handle, inv.handle,
		C.int(seq), C.int(stride), C.int(headsQ), C.int(offQ), C.int(headsK), C.int(offK),
		C.int(hd), C.int(half), C.int(posOffset), C.float(posDiv))
	if rc != 0 {
		return fmt.Errorf("metal: Recorder rope2 failed (%d)", int(rc))
	}
	return nil
}

func (r *Recorder) RoPEPairFixed(qkv, inv *DeviceBuffer, seq, stride, headsQ, offQ, headsK, offK, hd, half, posOffset int, posDiv float32) error {
	maxOff := max(offQ, offK)
	if offQ < 0 || offK < 0 || qkv.n < seq*stride || inv.n < half {
		return fmt.Errorf("metal: Recorder rope2 shape mismatch: qkv=%d (want %d at off %d) inv=%d (want %d)", qkv.n, seq*stride, maxOff, inv.n, half)
	}
	rc := C.mtl_recorder_rope2(r.handle, qkv.handle, inv.handle,
		C.int(seq), C.int(stride), C.int(headsQ), C.int(offQ), C.int(headsK), C.int(offK),
		C.int(hd), C.int(half), C.int(posOffset), C.float(posDiv))
	if rc != 0 {
		return fmt.Errorf("metal: Recorder rope2 failed (%d)", int(rc))
	}
	return nil
}

func (r *Recorder) RoPEPairVulkan(qkv, inv *DeviceBuffer, seq, stride, headsQ, offQ, headsK, offK, hd, half, posOffset int, posDiv float32) error {
	maxOff := max(offQ, offK)
	if offQ < 0 || offK < 0 || qkv.n < maxOff+seq*stride || inv.n < half { // want `conditional on positive representable geometry`
		return fmt.Errorf("vulkan: Recorder rope2 shape mismatch: qkv=%d (want %d at off %d) inv=%d (want %d)", qkv.n, seq*stride, maxOff, inv.n, half)
	}
	rc := C.vk_recorder_rope2(r.handle,
		(*C.uint32_t)(unsafe.Pointer(&rope2Spirv[0])), C.int(len(rope2Spirv)),
		qkv.handle, inv.handle,
		C.int(seq), C.int(stride), C.int(headsQ), C.int(offQ), C.int(headsK), C.int(offK),
		C.int(hd), C.int(half), C.int(posOffset), C.float(posDiv))
	if rc != 0 {
		return fmt.Errorf("vulkan: Recorder rope2 failed (%d)", int(rc))
	}
	return nil
}
