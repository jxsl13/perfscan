package ps6087transitive

import "ps6087facade"

func eligibleThroughFacade(gateProjection, upProjection, downProjection *ps6087facade.Linear, input *ps6087facade.Tensor) (*ps6087facade.Tensor, error) {
	ops := ps6087facade.NewExecutor()
	if ops.IsEager() {
		up, err := upProjection.Forward(input)
		if err != nil {
			return nil, err
		}
		gate, err := gateProjection.Forward(input)
		if err != nil {
			return nil, err
		}
		act := ops.SiLU(gate) // want `swiglu: configured fresh-owned ps6087cap.Linear.Forward result gate is consumed only by fresh-output ps6087cap.Executor.SiLU and that result only by fresh-output ps6087cap.Executor.Mul;.*implements or is assertable to ps6087cap.SwiGLUInPlaceFuser.FuseSwiGLUInPlace.*leaves both operands unchanged when unsupported.*advisory, no automatic fix`
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}
