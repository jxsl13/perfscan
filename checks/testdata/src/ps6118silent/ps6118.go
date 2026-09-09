package ps6118silent

type Accelerator struct{}
type HostOptimizer struct{}

func (Accelerator) LossAndGrad(parameters, batch []float32) (float32, []float32) {
	return 1, make([]float32, len(parameters))
}
func (HostOptimizer) Step(parameters, gradients []float32) {}
func observe(float32)                                      {}

func train(parameters, batch []float32, accelerator Accelerator, optimizer HostOptimizer) {
	for step := 0; step < 4; step++ {
		loss, gradients := accelerator.LossAndGrad(parameters, batch)
		optimizer.Step(parameters, gradients)
		observe(loss)
	}
}
