package ps6013

import "sync"

type Tensor struct {
	data []float32
}

func (t *Tensor) Storage() []float32 { return t.data }

func NewTensor(data []float32) *Tensor { return &Tensor{data: data} }

type DeviceBuffer struct{}
type DeviceTensor struct{}

type MetalDriver struct{}

func (*MetalDriver) Upload([]float32) *DeviceBuffer                           { return &DeviceBuffer{} }
func (*MetalDriver) DispatchScatter(*DeviceBuffer, *DeviceBuffer)             {}
func (*MetalDriver) ExecuteKernel(*DeviceBuffer)                              {}
func (*MetalDriver) Wait()                                                    {}
func (*MetalDriver) Download(*DeviceBuffer, []float32)                        {}
func (*MetalDriver) EnqueueAsync(*DeviceBuffer)                               {}
func (*MetalDriver) DownloadScalar(*DeviceBuffer, []float32)                  {}
func (*MetalDriver) DispatchReduction(*DeviceBuffer, *DeviceBuffer)           {}
func (*MetalDriver) DispatchElementwise(*DeviceBuffer, *DeviceBuffer)         {}
func (*MetalDriver) SynchronizeGPU()                                          {}
func (*MetalDriver) Readback(*DeviceBuffer, []float32)                        {}
func (*MetalDriver) CopyToHost(*DeviceBuffer, []float32)                      {}
func (*MetalDriver) ReadBuffer(*DeviceBuffer, *float32, int)                  {}
func (*MetalDriver) LaunchKernel(*DeviceBuffer, *DeviceBuffer, *DeviceBuffer) {}

func MetalEmbedBackward(metal *MetalDriver, indices, grad *Tensor, rows, dim int) *Tensor {
	indicesData := indices.Storage()
	gradData := grad.Storage()
	out := make([]float32, rows*dim)
	indicesBuffer := metal.Upload(indicesData)
	gradBuffer := metal.Upload(gradData)
	outBuffer := metal.Upload(out)
	metal.DispatchScatter(indicesBuffer, gradBuffer) // want "runtime-sized output out cross an accelerator kernel, synchronous wait, and full copy-back.*high-priority measurement target"
	metal.Wait()
	metal.Download(outBuffer, out)
	return NewTensor(out)
}

func MetalElementwiseRoundtrip(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	inputBuffer := metal.Upload(host)
	outputBuffer := metal.Upload(out)
	metal.DispatchElementwise(inputBuffer, outputBuffer) // want "runtime-sized output out cross an accelerator kernel, synchronous wait, and full copy-back"
	metal.SynchronizeGPU()
	metal.Readback(outputBuffer, out)
	return out
}

func (input *Tensor) MetalReceiverRoundtrip(metal *MetalDriver, n int) []float32 {
	host := input.Storage()
	out := make([]float32, 0, n)
	out = out[:n]
	inputBuffer := metal.Upload(host)
	outputBuffer := metal.Upload(out)
	metal.ExecuteKernel(inputBuffer) // want "runtime-sized output out cross an accelerator kernel, synchronous wait, and full copy-back"
	metal.Wait()
	metal.CopyToHost(outputBuffer, out)
	return out
}

func pointerAndLengthCopyback(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer) // want "runtime-sized output out cross an accelerator kernel, synchronous wait, and full copy-back"
	metal.Wait()
	metal.ReadBuffer(buffer, &out[0], len(out))
	return out
}

func noWait(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	inputBuffer := metal.Upload(host)
	outputBuffer := metal.Upload(out)
	metal.EnqueueAsync(inputBuffer)
	metal.DispatchElementwise(inputBuffer, outputBuffer)
	metal.Readback(outputBuffer, out)
	return out
}

func persistentDeviceContract(metal *MetalDriver, input *Tensor, n int) *DeviceTensor {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	metal.Wait()
	metal.Download(buffer, out)
	return &DeviceTensor{}
}

func noCopyback(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	metal.Wait()
	return out
}

func copybackDifferentObject(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	scalar := make([]float32, 1)
	buffer := metal.Upload(host)
	metal.DispatchReduction(buffer, buffer)
	metal.Wait()
	metal.DownloadScalar(buffer, scalar)
	return out
}

func partialCopyback(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	metal.Wait()
	metal.CopyToHost(buffer, out[:1])
	return out
}

func genericWaitGroupIsNotDeviceSync(metal *MetalDriver, input *Tensor, n int, wg *sync.WaitGroup) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	wg.Wait()
	metal.Download(buffer, out)
	return out
}

func copybackBeforeWait(metal *MetalDriver, input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	metal.Download(buffer, out)
	metal.Wait()
	return out
}

func fixedOutput(metal *MetalDriver, input *Tensor) []float32 {
	host := input.Storage()
	out := make([]float32, 16)
	buffer := metal.Upload(host)
	metal.ExecuteKernel(buffer)
	metal.Wait()
	metal.Download(buffer, out)
	return out
}

func noHostStorageAccess(metal *MetalDriver, input *Tensor, n int) []float32 {
	out := make([]float32, n)
	buffer := metal.Upload(input.data)
	metal.ExecuteKernel(buffer)
	metal.Wait()
	metal.Download(buffer, out)
	return out
}

func hostOnly(input *Tensor, n int) []float32 {
	host := input.Storage()
	out := make([]float32, n)
	copy(out, host)
	return out
}
