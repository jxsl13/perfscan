package ps6121

func parallelFor(n int, bodies ...func(start, end int)) {
	for _, body := range bodies {
		body(0, n)
	}
}
func notParallel(n int, body func(start, end int)) { body(0, n) }
func geluGrad(x, g float64) float64                { return x * g }
func reluGrad(x, g float64) float64                { return g }

// Faithful owner shape: function parameter, parallel band closure, band loop.
func activationBackward(xs, gs, ds []float64, grad func(float64, float64) float64) {
	parallelFor(len(ds), func(start, end int) {
		for i := start; i < end; i++ {
			ds[i] = grad(xs[i], gs[i]) // want `parallelFor captures source-monomorphic function parameter grad.*inspect the exact callback machine code.*owner candidate was rejected`
		}
	})
}
func geluBackward(xs, gs, ds []float64) { activationBackward(xs, gs, ds, geluGrad) }

// Multiple callers are accepted when the concrete function object is identical.
func shared(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for j := lo; j < hi; j++ {
			out[j] = operation(xs[j], 1) // want `parallelFor captures source-monomorphic function parameter operation`
		}
	})
}
func sharedA(x, out []float64) { shared(x, out, geluGrad) }
func sharedB(x, out []float64) { shared(x, out, geluGrad) }

// Different concrete callers are intentionally dynamic.
func different(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func differentA(x, out []float64) { different(x, out, geluGrad) }
func differentB(x, out []float64) { different(x, out, reluGrad) }

// A package-visible escape makes the helper reference surface incomplete.
func escaping(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}

var escapedHelper = escaping

func escapingCaller(x, out []float64) { escaping(x, out, geluGrad) }

func dynamic(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func dynamicCaller(x, out []float64, fn func(float64, float64) float64) { dynamic(x, out, fn) }

type gradients struct{}

func (gradients) apply(x, g float64) float64 { return x + g }
func methodValue(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func methodCaller(x, out []float64, g gradients) { methodValue(x, out, g.apply) }

func mutated(xs, out []float64, operation func(float64, float64) float64) {
	operation = geluGrad
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func mutatedCaller(x, out []float64) { mutated(x, out, geluGrad) }

func outsideLoop(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		out[lo] = operation(xs[lo], 1)
		for i := lo; i < hi; i++ {
			out[i]++
		}
	})
}
func outsideCaller(x, out []float64) { outsideLoop(x, out, geluGrad) }

func uninvoked(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		inner := func() {
			for i := lo; i < hi; i++ {
				out[i] = operation(xs[i], 1)
			}
		}
		_ = inner
	})
}
func uninvokedCaller(x, out []float64) { uninvoked(x, out, geluGrad) }

func dead(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		if false {
			for i := lo; i < hi; i++ {
				out[i] = operation(xs[i], 1)
			}
		}
	})
}
func deadCaller(x, out []float64) { dead(x, out, geluGrad) }

func immediateExit(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			break
			out[i] = operation(xs[i], 1)
		}
	})
}
func immediateCaller(x, out []float64) { immediateExit(x, out, geluGrad) }

func trivialFixed(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := 0; i < 2; i++ {
			out[i] = operation(xs[i], 1)
		}
		_, _ = lo, hi
	})
}
func trivialCaller(x, out []float64) { trivialFixed(x, out, geluGrad) }

func independentBound(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := 0; i < len(out); i++ {
			out[i] = operation(xs[i], 1)
		}
		_, _ = lo, hi
	})
}
func independentCaller(x, out []float64) { independentBound(x, out, geluGrad) }

func unconfigured(xs, out []float64, operation func(float64, float64) float64) {
	notParallel(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func unconfiguredCaller(x, out []float64) { unconfigured(x, out, geluGrad) }

func ambiguous(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	}, func(_, _ int) {})
}
func ambiguousCaller(x, out []float64) { ambiguous(x, out, geluGrad) }

func shadowedFanout(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor := func(n int, body func(int, int)) { body(0, n) }
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func shadowedCaller(x, out []float64) { shadowedFanout(x, out, geluGrad) }

func goroutineCall(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			go operation(xs[i], 1)
		}
	})
}
func goroutineCaller(x, out []float64) { goroutineCall(x, out, geluGrad) }

func extraFunctionCapture(xs, out []float64, operation, observe func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = observe(operation(xs[i], 1), 1)
		}
	})
}
func extraCaptureCaller(x, out []float64) { extraFunctionCapture(x, out, geluGrad, geluGrad) }

func genericGrad[T ~float64](x, g T) T { return x * g }
func genericArgument(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func genericArgumentCaller(x, out []float64) { genericArgument(x, out, genericGrad[float64]) }

func ExportedHelper(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func exportedHelperCaller(x, out []float64) { ExportedHelper(x, out, geluGrad) }

func alreadyDirect(xs, out []float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = geluGrad(xs[i], 1)
		}
	})
}

//perfscan:monomorphic-loop-callback-validated exact objdump evidence retained
func compilerValidated(xs, out []float64, operation func(float64, float64) float64) {
	parallelFor(len(out), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			out[i] = operation(xs[i], 1)
		}
	})
}
func compilerValidatedCaller(x, out []float64) { compilerValidated(x, out, geluGrad) }
