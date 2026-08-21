package ps6015

type Tensor struct {
	data []float32
}

func (t *Tensor) Storage() []float32   { return t.data }
func NewTensor(data []float32) *Tensor { return &Tensor{data: data} }

type DeviceTensor struct{}
type DeviceBuffer struct{}
type VulkanDriver struct{}

func (*VulkanDriver) Upload([]float32) *DeviceBuffer                { return &DeviceBuffer{} }
func (*VulkanDriver) DispatchReduction(*DeviceBuffer) *DeviceBuffer { return &DeviceBuffer{} }
func (*VulkanDriver) LaunchKernel(*DeviceBuffer) *DeviceBuffer      { return &DeviceBuffer{} }
func (*VulkanDriver) Wait()                                         {}
func (*VulkanDriver) Download(*DeviceBuffer) []float32              { return nil }
func (*VulkanDriver) EnqueueAsync(*DeviceBuffer)                    {}

const (
	OpAddBiasBackward = "OpAddBiasBackward"
	OpLayerNorm       = "OpLayerNorm"
	OpSoftmax         = "OpSoftmax"
	OpTranspose       = "OpTranspose"
	OpReduceMean      = "OpReduceMean"
	OpGELU            = "OpGELU"
	DTypeF32          = "F32"
	DTypeF16          = "F16"
	BackendCPU        = "CPU"
	BackendMetal      = "Metal"
)

func RegisterKernel(operation, dtype, backend string, implementation any) {}

func parallelTypedAddBiasBackwardF32() {}
func simdOptimizedLayerNormF32()       {}
func parallelTypedSoftmaxF16()         {}
func scalarReferenceTransposeF32()     {}
func vectorizedReduceMeanF32()         {}
func nativeMetalGELUF32()              {}

func init() {
	RegisterKernel(OpAddBiasBackward, DTypeF32, BackendCPU, parallelTypedAddBiasBackwardF32)
	RegisterKernel(OpLayerNorm, DTypeF32, BackendCPU, simdOptimizedLayerNormF32)
	RegisterKernel(OpSoftmax, DTypeF16, BackendCPU, parallelTypedSoftmaxF16)
	RegisterKernel(OpTranspose, DTypeF32, BackendCPU, scalarReferenceTransposeF32)
	RegisterKernel(OpReduceMean, DTypeF32, BackendCPU, vectorizedReduceMeanF32)
	RegisterKernel(OpGELU, DTypeF32, BackendMetal, nativeMetalGELUF32)
}
