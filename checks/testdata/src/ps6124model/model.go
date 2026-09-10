package model

import (
	"ps6124backend"
	"ps6124linear"
)

type Model struct{ Projection *linear.Linear }

func Load() (*Model, error) { return &Model{Projection: &linear.Linear{}}, nil }
func (m *Model) DecodeStep(ctx *backend.Context) { // want DecodeStep:"global backend effects: routers=ps6124backend.Default;writers="
	helper(m.Projection, ctx)
}
func helper(layer *linear.Linear, ctx *backend.Context) { // want helper:"global backend effects: routers=ps6124backend.Default;writers="
	layer.Forward(ctx)
}
