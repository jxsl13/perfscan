package metal

import "ps6071/backend"

type Backend struct{}

func (Backend) Name() string { return "metal" }

func cpuPrefers(op backend.Op, dtype backend.Dtype) (backend.Backend, bool) {
	cpu, ok := backend.Get(backend.CPU)
	if !ok {
		return nil, false
	}
	if _, has := cpu.Kernel(op, dtype); !has {
		return nil, false
	}
	return cpu, true
}

func binaryKernel(op backend.Op) backend.Kernel {
	return func(ctx *backend.Context, in []*backend.Tensor, attrs backend.Attrs) ([]*backend.Tensor, error) {
		a := in[0]
		if cpu, ok := cpuPrefers(op, a.Dtype()); ok {
			return backend.Execute(ctx.WithBackend(cpu).WithRecorder(nil), op, in, attrs) // want `binaryKernel selects a compile-time preferred backend through cpuPrefers and re-enters Execute`
		}
		return nil, nil
	}
}

func dynamicUnaryKernel(op backend.Op) backend.Kernel {
	return func(ctx *backend.Context, in []*backend.Tensor, attrs backend.Attrs) ([]*backend.Tensor, error) {
		if len(in) < 1024 {
			if cpu, ok := cpuPrefers(op, in[0].Dtype()); ok {
				return backend.Execute(ctx.WithBackend(cpu).WithRecorder(nil), op, in, attrs)
			}
		}
		return nil, nil
	}
}

func refFallback(op backend.Op) backend.Kernel {
	return func(ctx *backend.Context, in []*backend.Tensor, attrs backend.Attrs) ([]*backend.Tensor, error) {
		return backend.Execute(ctx.WithBackend(nil).WithRecorder(nil), op, in, attrs)
	}
}
