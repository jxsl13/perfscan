package ps6081alias

import (
	"ps6081owner/backend"
	"ps6081owner/linear"
	model "ps6081owner/types"
)

type Layer struct {
	Gate *linear.QuantLinear
	Up   *linear.QuantLinear
}

var escaped *model.Tensor
var escapedClosure func()

func (s *Layer) SafeAliases(ctx *backend.Context, x *model.Tensor) {
	pointerAlias := x
	valuesAlias := x.Values
	mapAlias := x.Meta
	receiverAlias := s
	weightAlias := s.Gate.Weight
	_, _, _, _ = valuesAlias, mapAlias, receiverAlias, weightAlias
	gate, err := s.Gate.Forward(ctx, pointerAlias)
	_, _ = gate, err
}

func (s *Layer) PointerMutation(ctx *backend.Context, x *model.Tensor) {
	pointerAlias := x
	gate, err := s.Gate.Forward(ctx, pointerAlias)
	pointerAlias.Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) SliceMutation(ctx *backend.Context, x *model.Tensor) {
	valuesAlias := x.Values
	gate, err := s.Gate.Forward(ctx, x)
	valuesAlias[0] = 1
	_, _ = gate, err
}

func (s *Layer) MapMutation(ctx *backend.Context, x *model.Tensor) {
	mapAlias := x.Meta
	gate, err := s.Gate.Forward(ctx, x)
	mapAlias["changed"] = 1
	_, _ = gate, err
}

func (s *Layer) ReceiverMutation(ctx *backend.Context, x *model.Tensor) {
	receiverAlias := s
	gate, err := s.Gate.Forward(ctx, x)
	receiverAlias.Up = receiverAlias.Gate
	_, _ = gate, err
}

func (s *Layer) WeightMutation(ctx *backend.Context, x *model.Tensor) {
	weightAlias := s.Gate.Weight
	gate, err := s.Gate.Forward(ctx, x)
	weightAlias.Tag = 1
	_, _ = gate, err
}

func (s *Layer) AliasRebinding(ctx *backend.Context, x *model.Tensor) {
	pointerAlias := x
	gate, err := s.Gate.Forward(ctx, pointerAlias)
	pointerAlias = &model.Tensor{}
	_, _ = gate, err
}

func (s *Layer) AliasEscape(ctx *backend.Context, x *model.Tensor) {
	pointerAlias := x
	gate, err := s.Gate.Forward(ctx, pointerAlias)
	escaped = pointerAlias
	_, _ = gate, err
}

func (s *Layer) ContainerMutation(ctx *backend.Context, x *model.Tensor) {
	container := []*model.Tensor{x}
	gate, err := s.Gate.Forward(ctx, x)
	container[0].Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) PostVarContainerMutation(ctx *backend.Context, x *model.Tensor) {
	gate, err := s.Gate.Forward(ctx, x)
	var container = []*model.Tensor{x}
	container[0].Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) PreRangeAliasMutation(ctx *backend.Context, x *model.Tensor) {
	var ranged *model.Tensor
	for _, alias := range x.Children {
		ranged = alias
		break
	}
	gate, err := s.Gate.Forward(ctx, x)
	ranged.Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) PostRangeAliasMutation(ctx *backend.Context, x *model.Tensor) {
	gate, err := s.Gate.Forward(ctx, x)
	for _, alias := range x.Children {
		alias.Values[0] = 1
	}
	_, _ = gate, err
}

func (s *Layer) CommaOKAliasMutation(ctx *backend.Context, x *model.Tensor) {
	boxed := any(x)
	alias, _ := boxed.(*model.Tensor)
	gate, err := s.Gate.Forward(ctx, x)
	alias.Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) MultiResultValueSpecAliasMutation(ctx *backend.Context, x *model.Tensor) {
	boxed := any(x)
	var alias, _ = boxed.(*model.Tensor)
	gate, err := s.Gate.Forward(ctx, x)
	alias.Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) MultiResultCallAliasMutation(ctx *backend.Context, x *model.Tensor) {
	var alias, _ = x.Alias()
	gate, err := s.Gate.Forward(ctx, x)
	alias.Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) IndexedStoreAliasMutation(ctx *backend.Context, x *model.Tensor) {
	container := make([]*model.Tensor, 1)
	container[0] = x
	gate, err := s.Gate.Forward(ctx, x)
	container[0].Values[0] = 1
	_, _ = gate, err
}

func (s *Layer) InterfaceRebinding(ctx *backend.Context, x *model.Tensor) {
	boxed := any(x)
	_ = boxed
	gate, err := s.Gate.Forward(ctx, x)
	boxed = nil
	_, _ = gate, err
}

func (s *Layer) ClosureEscape(ctx *backend.Context, x *model.Tensor) {
	capture := func() { x.Values[0] = 1 }
	gate, err := s.Gate.Forward(ctx, x)
	escapedClosure = capture
	_, _ = gate, err
}

func (s *Layer) ClosureInvocation(ctx *backend.Context, x *model.Tensor) {
	capture := func() { x.Values[0] = 1 }
	gate, err := s.Gate.Forward(ctx, x)
	capture()
	_, _ = gate, err
}

func (s *Layer) MethodValueInvocation(ctx *backend.Context, x *model.Tensor) {
	method := x.Ndim
	gate, err := s.Gate.Forward(ctx, x)
	method()
	_, _ = gate, err
}

func (s *Layer) CopyMutation(ctx *backend.Context, x *model.Tensor) {
	valuesAlias := x.Values
	gate, err := s.Gate.Forward(ctx, x)
	copy(valuesAlias, []float32{1})
	_, _ = gate, err
}

func (s *Layer) DeleteMutation(ctx *backend.Context, x *model.Tensor) {
	mapAlias := x.Meta
	gate, err := s.Gate.Forward(ctx, x)
	delete(mapAlias, "key")
	_, _ = gate, err
}

func (s *Layer) ClearMutation(ctx *backend.Context, x *model.Tensor) {
	mapAlias := x.Meta
	gate, err := s.Gate.Forward(ctx, x)
	clear(mapAlias)
	_, _ = gate, err
}

func (s *Layer) UnknownEffect(ctx *backend.Context, x *model.Tensor) {
	pointerAlias := x
	gate, err := s.Gate.Forward(ctx, x)
	unknown(pointerAlias)
	_, _ = gate, err
}

func unknown(*model.Tensor) {}
