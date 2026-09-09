package ps6118

type Tensor struct{}
type ScalarMetric struct{}
type Model struct{}
type Optimizer struct{}

func (*Model) LossAndGrad([]float32) (*ScalarMetric, []*Tensor, error) {
	return &ScalarMetric{}, []*Tensor{{}}, nil
}

func (*Optimizer) Step(func(*Tensor) *Tensor) error { return nil }

func observe(*ScalarMetric) {}
func consume(any)           {}

func fixed(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch) // want `configured cross-step accelerator boundary uploads 4096 parameter bytes and materializes 4096 dense-gradient bytes per step around host optimizer ps6118.Optimizer.Step; 8192 configured avoidable bytes/step \(32768 across 4 fixed iterations\)`
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func secondFixed(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch) // want `configured cross-step accelerator boundary uploads 4096 parameter bytes and materializes 4096 dense-gradient bytes per step`
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func faithfulOwnerReplay(model *Model, optimizer *Optimizer, batch []float32, parameterIndex map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, grads, err := model.LossAndGrad(batch) // want `configured cross-step accelerator boundary uploads 4096 parameter bytes and materializes 4096 dense-gradient bytes per step`
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return grads[parameterIndex[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func wrongCount(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 3; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func extraGradientConsumer(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		consume(gradients)
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func extraScalarConsumer(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		consume(loss)
		observe(loss)
	}
	return nil
}

func aliasGradient(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		alias := gradients
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return alias[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func wrongCallback(model *Model, optimizer *Optimizer, batch []float32) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(*Tensor) *Tensor { return gradients[0] }); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func uncheckedObjectiveError(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, _ := model.LossAndGrad(batch)
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

func ignoredOptimizerError(model *Model, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		_ = optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		})
		observe(loss)
	}
	return nil
}

type Objective interface {
	LossAndGrad([]float32) (*ScalarMetric, []*Tensor, error)
}

func dynamicObjective(model Objective, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}

type SliceModel struct{}

func (*SliceModel) LossAndGrad([]float32) ([]float32, []*Tensor, error) {
	return nil, nil, nil
}
func observeSlice([]float32) {}

func sliceScalar(model *SliceModel, optimizer *Optimizer, batch []float32, index map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(batch)
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[index[parameter]]
		}); err != nil {
			return err
		}
		observeSlice(loss)
	}
	return nil
}
