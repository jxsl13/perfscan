package ps6015

// Vulkan route replaced the slow scalar host fallback column sum.
func VulkanOpAddBiasBackwardF32(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	output := driver.DispatchReduction(device) // want "accelerator route addbiasbackward/f32 still cites a slow host/reference fallback.*high-priority"
	driver.Wait()
	return NewTensor(driver.Download(output))
}

// The slow scalar reference fallback motivated this Vulkan route, but a
// current hardware/shape crossover benchmark exists.
func VulkanOpLayerNormF32(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	output := driver.LaunchKernel(device)
	driver.Wait()
	return NewTensor(driver.Download(output))
}

// A different-dtype optimized registration does not invalidate this route.
// The GPU path replaced a slow scalar host fallback.
func VulkanOpSoftmaxF32(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	output := driver.LaunchKernel(device)
	driver.Wait()
	return NewTensor(driver.Download(output))
}

// A scalar reference registration is not an optimized host kernel.
// The GPU path replaced a slow scalar host fallback.
func VulkanOpTransposeF32(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	output := driver.LaunchKernel(device)
	driver.Wait()
	return NewTensor(driver.Download(output))
}

// No stale fallback rationale: silent.
func VulkanOpReduceMeanF32(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	output := driver.DispatchReduction(device)
	driver.Wait()
	return NewTensor(driver.Download(output))
}

// The GPU path replaced a slow scalar host fallback, but remains asynchronous.
func VulkanOpAddBiasBackwardF32Async(driver *VulkanDriver, input *Tensor) *Tensor {
	host := input.Storage()
	device := driver.Upload(host)
	driver.EnqueueAsync(device)
	return NewTensor(host)
}

// Persistent device contracts stay outside the audit even with stale rationale.
func VulkanOpAddBiasBackwardF32Resident(driver *VulkanDriver, input *DeviceTensor) *DeviceTensor {
	return input
}
