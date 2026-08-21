package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

const factBackendSource = `package backend

import "sync"

type Op int
type Dtype int
type Attrs any
type Tensor struct{ Dtype Dtype }
type Kernel func(*Context, []*Tensor, Attrs) ([]*Tensor, error)
type Backend interface { Kernel(Op, Dtype) (Kernel, bool) }
type Context struct{ Backend Backend }
func (c *Context) WithBackend(b Backend) *Context { return &Context{Backend: b} }

const CPU = "cpu"
var mu sync.RWMutex
var registry = map[string]Backend{}
var reference Backend

func Get(name string) (Backend, bool) {
	mu.RLock(); defer mu.RUnlock()
	b, ok := registry[name]
	return b, ok
}
func Reference() Backend {
	mu.RLock(); defer mu.RUnlock()
	return reference
}

func Execute(ctx *Context, op Op, inputs []*Tensor, attrs Attrs) ([]*Tensor, error) {
	dtype := inputs[0].Dtype
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
`

const factMetalSource = `package metal

import "factcorpus/backend"

func cpuPrefers(op backend.Op, dtype backend.Dtype) (backend.Backend, bool) {
	cpu, ok := backend.Get(backend.CPU)
	if !ok { return nil, false }
	if _, has := cpu.Kernel(op, dtype); !has { return nil, false }
	return cpu, true
}

func binaryKernel(op backend.Op) backend.Kernel {
	return func(ctx *backend.Context, in []*backend.Tensor, attrs backend.Attrs) ([]*backend.Tensor, error) {
		if cpu, ok := cpuPrefers(op, in[0].Dtype); ok {
			return backend.Execute(ctx.WithBackend(cpu), op, in, attrs)
		}
		return nil, nil
	}
}
`

func TestAnalyzerFactsFlowFromDependency(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module factcorpus\n\ngo 1.25\n")
	write("backend/backend.go", factBackendSource)
	write("metal/metal.go", factMetalSource)

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()

	var stdout, stderr bytes.Buffer
	code := Run([]*lint.Check{checks.PS6071}, Options{
		Patterns: []string{"./metal"},
		Checks:   "PS6071",
		MaxLevel: lint.LevelAggressive,
		ExitZero: true,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	output := stdout.String() + stderr.String()
	if code != 0 {
		t.Fatalf("fact-backed run returned %d:\n%s", code, output)
	}
	if !strings.Contains(output, "binaryKernel selects a compile-time preferred backend") || !strings.Contains(output, "PS6071") {
		t.Fatalf("dependency facts did not reach selected implementation:\n%s", output)
	}
	if strings.Contains(output, "Execute performs") {
		t.Fatalf("dependency diagnostics must stay scoped out when only ./metal is requested:\n%s", output)
	}
}
