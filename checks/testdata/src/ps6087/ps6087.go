package ps6087

import (
	c "ps6087cap"
	. "ps6087dotcap"
	_ "ps6087other"
)

var globalOps c.Executor

func eligible(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
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

func outsideGuard(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	gate, _ := gateProjection.Forward(input)
	act := ops.SiLU(gate)
	up, _ := upProjection.Forward(input)
	product := ops.Mul(act, up)
	return downProjection.Forward(product)
}

func negativeGuard(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if !ops.IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		up, _ := upProjection.Forward(input)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func differentProvider(first, second c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if first.IsEager() {
		observeProvider(first)
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := first.SiLU(gate)
		product := second.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func reassignedProvider(ops, replacement c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		ops = replacement
		up, _ := upProjection.Forward(input)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func reassignedBeforeActivation(ops, replacement c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		ops = replacement
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		up, _ := upProjection.Forward(input)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func observeProvider(c.Executor) {}

func providerReferenceBetweenCalls(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		observeProvider(ops)
		up, _ := upProjection.Forward(input)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func capturedProviderRebind(ops, replacement c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	replace := func() { ops = replacement }
	if ops.IsEager() {
		replace()
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func aliasedProviderRebind(ops, replacement c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	pointer := &ops
	if ops.IsEager() {
		*pointer = replacement
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func activationExtraArgument(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLUWithMode(gate, 1)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func binaryExtraArgument(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.MulScaled(act, up, 2)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func panicBeforeActivation(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		panic("unreachable fusion")
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func packageGlobalProvider(gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if globalOps.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := globalOps.SiLU(gate)
		product := globalOps.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func dotImportedGlobalProvider(gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if GlobalOps.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := GlobalOps.SiLU(gate)
		product := GlobalOps.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func touch(*c.Executor) {}

func exposedProvider(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		touch(&ops)
		up, _ := upProjection.Forward(input)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func consume(*c.Tensor) {}

func reusedGate(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		consume(gate)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func reusedActivation(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		consume(act)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func reusedProduct(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		consume(product)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func capturedGate(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		deferred := func() { consume(gate) }
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		deferred()
		return downProjection.Forward(product)
	}
	return nil, nil
}

func nestedActivationOperand(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(c.Clone(gate))
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func nestedBinaryOperand(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, c.Clone(up))
		return downProjection.Forward(product)
	}
	return nil, nil
}

func publicGate(ops c.Executor, upProjection, downProjection *c.Linear, gate *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(gate)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func pairProducer(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, retained, _ := gateProjection.ForwardPair(input)
		consume(retained)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func pairActivation(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act, retained, _ := ops.SiLUPair(gate)
		product := ops.Mul(act, up)
		consume(retained)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func pairBinary(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product, retained, _ := ops.MulPair(act, up)
		consume(retained)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func soleSelectorUse(ops c.Executor, gateProjection, upProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return product.View(), nil
	}
	return nil, nil
}

func provider() c.Executor { return nil }

func effectfulReceiver(gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if provider().IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := provider().SiLU(gate)
		up, _ := upProjection.Forward(input)
		product := provider().Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func indexedReceiver(providers []c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if providers[0].IsEager() {
		gate, _ := gateProjection.Forward(input)
		act := providers[0].SiLU(gate)
		up, _ := upProjection.Forward(input)
		product := providers[0].Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func deadPath(ops c.Executor, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		return nil, nil
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}

func concreteIncapable(ops *c.ConcreteOps, gateProjection, upProjection, downProjection *c.Linear, input *c.Tensor) (*c.Tensor, error) {
	if ops.IsEager() {
		up, _ := upProjection.Forward(input)
		gate, _ := gateProjection.Forward(input)
		act := ops.SiLU(gate)
		product := ops.Mul(act, up)
		return downProjection.Forward(product)
	}
	return nil, nil
}
