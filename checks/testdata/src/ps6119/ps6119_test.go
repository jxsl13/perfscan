package ps6119

import "testing"

type Weight struct{}
type Device struct{}

func (Device) Launch(input []float32, weight *Weight, output []float32) {}
func (*Weight) Dispatch(input, output []float32)                        {}
func upload() *Weight                                                   { return &Weight{} }

func BenchmarkDirectRange(b *testing.B) {
	d := Device{}
	w := upload()
	in, out := []float32{1}, []float32{0}
	for range b.N {
		d.Launch(in, w, out) // want "accelerator benchmark repeats one loop-invariant weight handle"
	}
}

func BenchmarkLoopReceiver(b *testing.B) {
	w := upload()
	in, out := []float32{1}, []float32{0}
	for b.Loop() {
		w.Dispatch(in, out) // want "accelerator benchmark repeats one loop-invariant weight handle"
	}
}

func BenchmarkOwnerClosureAndRun(b *testing.B) {
	d := Device{}
	w := upload()
	in, out := []float32{1}, []float32{0}
	run := func(b *testing.B) {
		d.Launch(in, w, out)
		if len(out) == 0 {
			b.Fatal("impossible")
		}
	}
	b.Run("resident", func(b *testing.B) {
		for range b.N {
			run(b) // want "accelerator benchmark repeats one loop-invariant weight handle"
		}
	})
}

func BenchmarkFixedIndex(b *testing.B) {
	d := Device{}
	weights := []*Weight{upload(), upload()}
	in, out := []float32{1}, []float32{0}
	for range b.N {
		d.Launch(in, weights[0], out) // want "accelerator benchmark repeats one loop-invariant weight handle"
	}
}

func BenchmarkRotating(b *testing.B) {
	d := Device{}
	weights := []*Weight{upload(), upload()}
	in, out := []float32{1}, []float32{0}
	for i := range b.N {
		d.Launch(in, weights[i%len(weights)], out)
	}
}

func BenchmarkFreshAndMutated(b *testing.B) {
	d := Device{}
	w := upload()
	in, out := []float32{1}, []float32{0}
	for range b.N {
		w = upload()
		d.Launch(in, w, out)
	}
}

func BenchmarkAddressed(b *testing.B) {
	d := Device{}
	w := upload()
	in, out := []float32{1}, []float32{0}
	for range b.N {
		_ = &w
		d.Launch(in, w, out)
	}
}

func invoke(w *Weight) { Device{}.Launch(nil, w, nil) }

var shared = upload()

func replaceShared() { shared = upload() }
func deadInvoke(w *Weight) {
	if false {
		invoke(w)
	}
}

func BenchmarkHelper(b *testing.B) {
	w := upload()
	for i := 0; i < b.N; i++ {
		invoke(w) // want "accelerator benchmark repeats one loop-invariant weight handle"
	}
}

func BenchmarkTwoFixedWeights(b *testing.B) {
	weights := []*Weight{upload(), upload()}
	for b.Loop() {
		invoke(weights[0])
		invoke(weights[1])
	}
}

func BenchmarkUncalledNested(b *testing.B) {
	w := upload()
	run := func() { _ = func() { invoke(w) } }
	for b.Loop() {
		run()
	}
}

func BenchmarkGlobalReplacement(b *testing.B) {
	for b.Loop() {
		replaceShared()
		invoke(shared)
	}
}

func BenchmarkImmediateBreak(b *testing.B) {
	w := upload()
	for b.Loop() {
		invoke(w)
		break
	}
}

func BenchmarkDeadDispatch(b *testing.B) {
	w := upload()
	for b.Loop() {
		if false {
			invoke(w)
		}
	}
}

func BenchmarkDeadSubbenchmark(b *testing.B) {
	w := upload()
	if false {
		b.Run("dead", func(b *testing.B) {
			for b.Loop() {
				invoke(w)
			}
		})
	}
}

func BenchmarkDeadHelper(b *testing.B) {
	w := upload()
	for b.Loop() {
		deadInvoke(w)
	}
}

func BenchmarkDeferred(b *testing.B) {
	w := upload()
	for b.Loop() {
		defer invoke(w)
	}
}

func BenchmarkReboundClosure(b *testing.B) {
	w := upload()
	run := func() { invoke(w) }
	n := 0
	run, n = func() {}, 1
	_ = n
	for b.Loop() {
		run()
	}
}

func BenchmarkDeadClosure(b *testing.B) {
	w := upload()
	run := func() {
		if false {
			invoke(w)
		}
	}
	for b.Loop() {
		run()
	}
}
func BenchmarkDeferredClosure(b *testing.B) {
	w := upload()
	run := func() { defer invoke(w) }
	for b.Loop() {
		run()
	}
}
func BenchmarkAddressedClosure(b *testing.B) {
	w := upload()
	run := func() { invoke(w) }
	p := &run
	*p = func() {}
	for b.Loop() {
		run()
	}
}
func hook(fn func())          { fn() }
func captureInvoke(w *Weight) { hook(func() { w.Dispatch(nil, nil) }); invoke(w) }
func BenchmarkMutatingHelperCapture(b *testing.B) {
	w := upload()
	for b.Loop() {
		captureInvoke(w)
	}
}
func BenchmarkDeferredMutationInHelper(b *testing.B) {
	w := upload()
	run := func() { defer w.Dispatch(nil, nil); invoke(w) }
	for b.Loop() {
		run()
	}
}
