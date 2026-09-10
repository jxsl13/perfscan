package ps6124

import (
	"ps6124backend"
	"ps6124model"
	"testing"
)

func baseline() { // want baseline:"global backend effects: routers=ps6124backend.Default;writers="
	model, _ := model.Load()
	run := func() {
		cpu, _ := backend.Get(backend.CPU)
		ctx := backend.NewContext().WithBackend(cpu)
		model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
	}
	run()
	run()
}

func fixed() { // want fixed:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	run := func() {
		cpu, _ := backend.Get(backend.CPU)
		ctx := backend.NewContext().WithBackend(cpu)
		model.DecodeStep(ctx)
	}
	run()
}

func late() { // want late:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	model, _ := model.Load()
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func unrelated() {
	model, _ := model.Load()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	_ = model
	_ = ctx
}

func conditionalMutation(cond bool) { // want conditionalMutation:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	if cond {
		backend.SetPreference(backend.GPU)
	}
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func mutatePreference() { // want mutatePreference:"global backend effects: routers=;writers=ps6124backend.SetPreference"
	backend.SetPreference(backend.GPU)
}

func helperMutation() { // want helperMutation:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	mutatePreference()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func snapshotOverwrite() { // want snapshotOverwrite:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	previous = nil
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx)
}

func indirectUnknown(callback func()) { // want indirectUnknown:"global backend effects: routers=ps6124backend.Default;writers="
	model, _ := model.Load()
	callback()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx)
}

func capturedReassign() { // want capturedReassign:"global backend effects: routers=ps6124backend.Default;writers="
	loaded, _ := model.Load()
	loaded, _ = model.Load()
	run := func() {
		cpu, _ := backend.Get(backend.CPU)
		ctx := backend.NewContext().WithBackend(cpu)
		loaded.DecodeStep(ctx)
	}
	run()
}

func tupleReuse() { // want tupleReuse:"global backend effects: routers=ps6124backend.Default;writers="
	loaded, err := model.Load()
	loaded, err = model.Load()
	_ = err
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	loaded.DecodeStep(ctx)
}

func deadRoute() {
	model, _ := model.Load()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	if false {
		model.DecodeStep(ctx)
	}
}

func parallelPin(t *testing.T) { // want parallelPin:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	t.Parallel()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func snapshotAliasWrite() { // want snapshotAliasWrite:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	alias := previous
	alias[0] = backend.GPU
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func contextReassign() { // want contextReassign:"global backend effects: routers=ps6124backend.Default;writers="
	model, _ := model.Load()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	ctx = backend.NewContext()
	model.DecodeStep(ctx)
}

func ignore(func()) {}

func ignoredCallbackWriter() { // want ignoredCallbackWriter:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	ignore(func() { backend.SetPreference(backend.GPU) })
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx)
}

func nestedUninvokedWriter() { // want nestedUninvokedWriter:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	unused := func() { backend.SetPreference(backend.GPU) }
	_ = unused
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx)
}

func liveWriterDeadSecond() { // want liveWriterDeadSecond:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	run := func() { backend.SetPreference(backend.GPU) }
	run()
	if false {
		run()
	}
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}

func noopThenWriter() { // want noopThenWriter:"global backend effects: routers=ps6124backend.Default;writers=ps6124backend.SetPreference"
	previous := backend.Preference()
	backend.SetPreference(backend.CPU)
	defer backend.SetPreference(previous...)
	model, _ := model.Load()
	run := func() {}
	run = func() { backend.SetPreference(backend.GPU) }
	run()
	cpu, _ := backend.Get(backend.CPU)
	ctx := backend.NewContext().WithBackend(cpu)
	model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}
