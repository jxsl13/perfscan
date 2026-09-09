package ps6081review

import (
	"ps6081owner/backend"
	"ps6081owner/linear"
	model "ps6081owner/types"
)

type Layer struct {
	Gate *linear.QuantLinear
	Up   *linear.QuantLinear
	Down *linear.QuantLinear
}

type RepeatedNode struct{ Node int }
type RepeatedA struct{ B *RepeatedB }
type RepeatedB struct{ A int }

type RepeatedReceiver struct {
	Weight  *model.Weight
	Backend backend.Backend
	Node    *RepeatedNode
	A       *RepeatedA
}

func (r *RepeatedReceiver) RepeatedForward(_ any, input *model.Tensor) (*model.Tensor, error) {
	return input, nil
}

// InputRebinding is a permanent regression for owner input replacement after
// the first producer. A contract must fail closed even if the replacement is
// type-compatible and the remainder of the flow has the configured shape.
func (s *Layer) InputRebinding(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) {
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}
	x = &model.Tensor{}
	projectionBackend := backend.Default()
	var up *model.Tensor
	if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
		if fuser.FuseSwiGLUInPlace(gate, up) {
			return s.Down.Forward(ctx, gate)
		}
	}
	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// ReceiverWeightMutation covers replacement of the owner receiver path and
// therefore of its receiver-held weight identity between sibling producers.
func (s *Layer) ReceiverWeightMutation(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) {
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}
	s.Up = s.Gate
	projectionBackend := backend.Default()
	var up *model.Tensor
	if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
		if fuser.FuseSwiGLUInPlace(gate, up) {
			return s.Down.Forward(ctx, gate)
		}
	}
	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// AliasAndFunctionHook covers both a reference alias of the participating
// input and an unconfigured function-value hook that can observe or mutate it.
func (s *Layer) AliasAndFunctionHook(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) {
	alias := x
	gate, err := s.Gate.Forward(ctx, alias)
	if err != nil {
		return nil, err
	}
	var hook func(*model.Tensor)
	hook(alias)
	projectionBackend := backend.Default()
	var up *model.Tensor
	if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
		up, err = s.Up.Forward(ctx, alias)
		if err != nil {
			return nil, err
		}
		if fuser.FuseSwiGLUInPlace(gate, up) {
			return s.Down.Forward(ctx, gate)
		}
	}
	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, alias)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// BeforeDefinition keeps the consumer-before-producer ordering regression
// visible: the configured transform occurs before the first producer result.
func (s *Layer) BeforeDefinition(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) {
	projectionBackend := backend.Default()
	var gate *model.Tensor
	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	gate, err = s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}
	var up *model.Tensor
	if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
		if fuser.FuseSwiGLUInPlace(gate, up) {
			return s.Down.Forward(ctx, gate)
		}
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// OptionalTransform preserves the complete syntactic owner shape while
// allowing the composite path to bypass the configured SiLU transform.
func (s *Layer) OptionalTransform(ctx *backend.Context, x *model.Tensor, runTransform bool) (*model.Tensor, error) {
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}
	projectionBackend := backend.Default()
	var up *model.Tensor
	if ctx != nil && ctx.Recorder == nil && ctx.Backend != nil && ctx.Backend.Name() == projectionBackend.Name() {
		if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
			up, err = s.Up.Forward(ctx, x)
			if err != nil {
				return nil, err
			}
			if fuser.FuseSwiGLUInPlace(gate, up) {
				return s.Down.Forward(ctx, gate)
			}
		}
	}
	var act []*model.Tensor
	if runTransform {
		act, err = backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
		if err != nil {
			return nil, err
		}
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// ReceiverFields is otherwise shaped like the target and is analyzed under
// contracts with nonexistent, swapped, same-storage, or differing receiver
// field roles. None of those contradictory contracts may produce a report.
func (s *Layer) ReceiverFields(ctx *backend.Context, x *model.Tensor) (*model.Tensor, error) {
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return nil, err
	}
	projectionBackend := backend.Default()
	var up *model.Tensor
	if fuser, ok := projectionBackend.(backend.SwiGLUInPlaceFuser); ok {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
		if fuser.FuseSwiGLUInPlace(gate, up) {
			return s.Down.Forward(ctx, gate)
		}
	}
	act, err := backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	if err != nil {
		return nil, err
	}
	if up == nil {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return nil, err
		}
	}
	h, err := backend.Execute(ctx, backend.OpMul, []*model.Tensor{act[0], up}, nil)
	if err != nil {
		return nil, err
	}
	return s.Down.Forward(ctx, h[0])
}

// DeadRoutes and UncalledClosure make the route reachability regressions
// explicit in source. Neither body is a live path to the configured helper.
func DeadRoutes(n, work int) {
	if false {
		deadHelper(n, work, func(int, int) {})
	}
	_ = func() { deadHelper(n, work, func(int, int) {}) }
}

func AfterReturn(n, work int) {
	return
	deadHelper(n, work, func(int, int) {})
}

func AfterPanic(n, work int) {
	panic("stop")
	deadHelper(n, work, func(int, int) {})
}

func AfterTrueReturn(n, work int) {
	if true {
		return
	}
	deadHelper(n, work, func(int, int) {})
}

func RouteProducerAfterReturn(n, work int) {
	return
	routeLive(n, work)
}

func RouteProducerAfterPanic(n, work int) {
	panic("stop")
	routeLive(n, work)
}

func RouteProducerAfterTrueReturn(n, work int) {
	if true {
		return
	}
	routeLive(n, work)
}

func RouteTerminalAfterReturn(n, work int) { routeDeadReturn(n, work) }
func RouteTerminalAfterPanic(n, work int)  { routeDeadPanic(n, work) }
func RouteTerminalAfterTrueReturn(n, work int) {
	routeDeadTrueReturn(n, work)
}

func routeLive(n, work int) { deadHelper(n, work, func(int, int) {}) }

func routeDeadReturn(n, work int) {
	return
	deadHelper(n, work, func(int, int) {})
}

func routeDeadPanic(n, work int) {
	panic("stop")
	deadHelper(n, work, func(int, int) {})
}

func routeDeadTrueReturn(n, work int) {
	if true {
		return
	}
	deadHelper(n, work, func(int, int) {})
}

func OptionalRouteProducer(n, work int, useRoute bool) (*model.Tensor, error) {
	if useRoute {
		output := &model.Tensor{}
		deadHelper(n, work, func(int, int) {})
		return output, nil
	}
	return &model.Tensor{}, nil
}

func OptionalNilSuccessRouteProducer(n, work int, bypass bool) (*model.Tensor, error) {
	var err error
	if bypass {
		return nil, err
	}
	deadHelper(n, work, func(int, int) {})
	return &model.Tensor{}, nil
}

func DeferredRouteProducer(n, work int) (*model.Tensor, error) {
	defer deadHelper(n, work, func(int, int) {})
	return &model.Tensor{}, nil
}

func RecursiveAsyncRouteProducer(n, work int) (*model.Tensor, error) {
	return asyncRouteWrapper(n, work)
}

func asyncRouteWrapper(n, work int) (*model.Tensor, error) {
	go deadHelper(n, work, func(int, int) {})
	return &model.Tensor{}, nil
}

func (s *Layer) OwnerAfterReturn(ctx *backend.Context, x *model.Tensor) {
	return
	_, _ = s.Gate.Forward(ctx, x)
}

func (s *Layer) OwnerAfterPanic(ctx *backend.Context, x *model.Tensor) {
	panic("stop")
	_, _ = s.Gate.Forward(ctx, x)
}

func (s *Layer) OwnerAfterTrueReturn(ctx *backend.Context, x *model.Tensor) {
	if true {
		return
	}
	_, _ = s.Gate.Forward(ctx, x)
}

func (s *Layer) SplitBranches(ctx *backend.Context, x *model.Tensor, choose bool) {
	var gate *model.Tensor
	if choose {
		gate, _ = s.Gate.Forward(ctx, x)
	} else {
		_, _ = backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
	}
}

func (s *Layer) TransformAfterTerminatingBranch(ctx *backend.Context, x *model.Tensor) {
	gate, _ := s.Gate.Forward(ctx, x)
	if true {
		return
	}
	_, _ = backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
}

func (s *Layer) OptionalGate(ctx *backend.Context, x *model.Tensor, choose bool) {
	var gate *model.Tensor
	if choose {
		gate, _ = s.Gate.Forward(ctx, x)
	}
	_, _ = backend.Execute(ctx, backend.OpSiLU, []*model.Tensor{gate}, nil)
}

func (s *Layer) OptionalUp(ctx *backend.Context, x *model.Tensor, choose bool) {
	gate, _ := s.Gate.Forward(ctx, x)
	var up *model.Tensor
	if choose {
		up, _ = s.Up.Forward(ctx, x)
	}
	_, _ = backend.Execute(ctx, backend.OpMul, []*model.Tensor{gate, up}, nil)
}

func (s *Layer) OptionalEarlyUp(ctx *backend.Context, x *model.Tensor, choose bool, fuser backend.SwiGLUInPlaceFuser) {
	gate, _ := s.Gate.Forward(ctx, x)
	var up *model.Tensor
	if choose {
		up, _ = s.Up.Forward(ctx, x)
	}
	if fuser.FuseSwiGLUInPlace(gate, up) {
		return
	}
}

func (s *Layer) OptionalFallback(ctx *backend.Context, x *model.Tensor, eager, fallback bool) {
	gate, err := s.Gate.Forward(ctx, x)
	if err != nil {
		return
	}
	var up *model.Tensor
	if eager {
		up, err = s.Up.Forward(ctx, x)
		if err != nil {
			return
		}
	}
	if fallback {
		if up == nil {
			up, err = s.Up.Forward(ctx, x)
			if err != nil {
				return
			}
		}
	}
	_, _ = backend.Execute(ctx, backend.OpMul, []*model.Tensor{gate, up}, nil)
}

func deadHelper(int, int, func(int, int)) {}
