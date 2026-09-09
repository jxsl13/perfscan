package ps6118owner

type Tensor struct{}
type ScalarLoss struct{}
type GradFn func(*Tensor) *Tensor
type GPT struct{}
type AdamF32 struct{}

func (*GPT) LossAndGrad([]int, *Tensor) (*ScalarLoss, []*Tensor, error) {
	return &ScalarLoss{}, []*Tensor{{}}, nil
}
func (*AdamF32) Step(GradFn) error { return nil }
func observe(*ScalarLoss)          {}

func train(model *GPT, optimizer *AdamF32, tokens []int, targets *Tensor, parameterIndex map[*Tensor]int) error {
	for step := 0; step < 4; step++ {
		loss, gradients, err := model.LossAndGrad(tokens, targets) // want `configured cross-step accelerator boundary uploads 4096 parameter bytes and materializes 4096 dense-gradient bytes per step`
		if err != nil {
			return err
		}
		if err := optimizer.Step(func(parameter *Tensor) *Tensor {
			return gradients[parameterIndex[parameter]]
		}); err != nil {
			return err
		}
		observe(loss)
	}
	return nil
}
