package backend

import (
	"sync"
	"sync/atomic"
)

type Op int
type Dtype int
type Attrs any
type Tensor struct{ dtype Dtype }

func (t *Tensor) Dtype() Dtype { return t.dtype }

type Kernel func(*Context, []*Tensor, Attrs) ([]*Tensor, error)

type Backend interface {
	Name() string
	Kernel(Op, Dtype) (Kernel, bool)
}

type Context struct {
	Backend    Backend
	Recorder   any
	opBackends map[Op]string
}

func (c *Context) WithBackend(b Backend) *Context { return &Context{Backend: b, Recorder: c.Recorder} }
func (c *Context) WithRecorder(r any) *Context    { return &Context{Backend: c.Backend, Recorder: r} }

const CPU = "cpu"

var (
	regMu     sync.RWMutex
	registry  = map[string]Backend{}
	reference Backend
)

func Get(name string) (Backend, bool) { // want Get:"registry-accessor"
	regMu.RLock()
	defer regMu.RUnlock()
	b, ok := registry[name]
	return b, ok
}

func Reference() Backend { // want Reference:"registry-accessor"
	regMu.RLock()
	defer regMu.RUnlock()
	return reference
}

func Execute(ctx *Context, op Op, inputs []*Tensor, attrs Attrs) ([]*Tensor, error) { // want Execute:"backend-dispatcher" `Execute performs 3 synchronized registry reads, 5 virtual Kernel probes over stable \(op, dtype\), and 2 route-context derivations`
	dtype := Dtype(0)
	if len(inputs) > 0 {
		dtype = inputs[0].Dtype()
	}
	if name, routed := ctx.opBackends[op]; routed {
		if rb, ok := Get(name); ok && rb != ctx.Backend {
			if _, has := rb.Kernel(op, dtype); has {
				ctx = ctx.WithBackend(rb)
			}
		}
	}
	k, ok := ctx.Backend.Kernel(op, dtype)
	if !ok {
		cpu, _ := Get(CPU)
		if _, has := cpu.Kernel(op, dtype); has {
			k, _ = cpu.Kernel(op, dtype)
		} else {
			ref := Reference()
			k, _ = ref.Kernel(op, dtype)
		}
		ctx = ctx.WithBackend(cpu)
	}
	return k(ctx, inputs, attrs)
}

// Lookup-only helpers do not form a dispatcher finding.
func LookupOnly(name string) Backend {
	b, _ := Get(name)
	return b
}

func DispatchMutable(ctx *Context, op Op, dtype Dtype) Kernel {
	first, _ := ctx.Backend.Kernel(op, dtype)
	op++
	second, _ := ctx.Backend.Kernel(op, dtype)
	_, _ = Get(CPU)
	_, _ = Get(CPU)
	_ = ctx.WithBackend(ctx.Backend)
	if second != nil {
		return second
	}
	return first
}

type routeEntry struct{ kernel Kernel }
type routeTable struct{ ptr atomic.Pointer[routeEntry] }

var kernelRouteCache routeTable
var registryGeneration atomic.Uint64

func ExecuteCached(ctx *Context, op Op, dtype Dtype) Kernel { // want ExecuteCached:"backend-dispatcher"
	_ = kernelRouteCache.ptr.Load()
	_ = registryGeneration.Load()
	first, _ := ctx.Backend.Kernel(op, dtype)
	second, _ := ctx.Backend.Kernel(op, dtype)
	_, _ = Get(CPU)
	_, _ = Get(CPU)
	_ = ctx.WithBackend(ctx.Backend)
	if second != nil {
		return second
	}
	return first
}

//perfscan:backend-resolution-cache-validated exact identity and invalidation reviewed.
func DispatchValidated(ctx *Context, op Op, dtype Dtype) Kernel { // want DispatchValidated:"backend-dispatcher"
	first, _ := ctx.Backend.Kernel(op, dtype)
	second, _ := ctx.Backend.Kernel(op, dtype)
	_, _ = Get(CPU)
	_, _ = Get(CPU)
	_ = ctx.WithBackend(ctx.Backend)
	if second != nil {
		return second
	}
	return first
}

type concrete struct{}

func (concrete) Name() string                    { return "concrete" }
func (concrete) Kernel(Op, Dtype) (Kernel, bool) { return nil, false }

func DispatchConcrete(ctx *Context, op Op, dtype Dtype) {
	c := concrete{}
	_, _ = c.Kernel(op, dtype)
	_, _ = c.Kernel(op, dtype)
	_, _ = Get(CPU)
	_, _ = Get(CPU)
	_ = ctx.WithBackend(c)
}
