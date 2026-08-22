package ps6018

import "testing"

func runControl()   {}
func runCandidate() {}
func setup()        {}

// Fixed-count route evidence: -benchtime=100x -count=7; reject max/min above
// the frozen stability ceiling.
func BenchmarkRouteDirectUnderwarmed(b *testing.B) {
	runControl()
	runCandidate()
	b.ResetTimer() // want "fixed-count route benchmark warms runCandidate and runControl only once before ResetTimer"
	for range b.N {
		runControl()
		runCandidate()
	}
}

// Fixed-count route evidence: -benchtime=100x -count=7; max/min stability.
func BenchmarkRouteDirectRepeatedWarmup(b *testing.B) {
	for range 10 {
		runControl()
		runCandidate()
	}
	b.ResetTimer()
	for range b.N {
		runControl()
		runCandidate()
	}
}

// Fixed-count route evidence: -benchtime=100x -count=7; max/min stability.
// The eager initialization barrier proves there is no lazy initialization.
func BenchmarkRouteDocumentedBarrier(b *testing.B) {
	runControl()
	runCandidate()
	b.ResetTimer()
	for range b.N {
		runControl()
		runCandidate()
	}
}

// No controlled repetition claim: one warmup is not audited.
func BenchmarkRouteOrdinary(b *testing.B) {
	runControl()
	runCandidate()
	b.ResetTimer()
	for range b.N {
		runControl()
		runCandidate()
	}
}

// Fixed-count leadership evidence: -benchtime=100x -count=7, max/min ceiling.
func BenchmarkLeadershipSubbenchUnderwarmed(b *testing.B) {
	b.Run("control", func(b *testing.B) {
		setup()
		runControl()
		b.ResetTimer() // want "fixed-count route benchmark warms b.Run arm[(]s[)] candidate, control only once before ResetTimer"
		for range b.N {
			runControl()
		}
	})
	b.Run("candidate", func(b *testing.B) {
		setup()
		runCandidate()
		b.ResetTimer()
		for range b.N {
			runCandidate()
		}
	})
}

// Fixed-count leadership evidence: max/min stability ceiling.
func BenchmarkLeadershipSubbenchRepeated(b *testing.B) {
	b.Run("CPU", func(b *testing.B) {
		for range 10 {
			runControl()
		}
		b.ResetTimer()
		for range b.N {
			runControl()
		}
	})
	b.Run("Vulkan", func(b *testing.B) {
		for range 10 {
			runCandidate()
		}
		b.ResetTimer()
		for range b.N {
			runCandidate()
		}
	})
}

func benchmarkRouteArm(b *testing.B, arm func()) {
	arm()
	b.ResetTimer() // want "fixed-count route benchmark helper benchmarkRouteArm invokes its selected arm only once before ResetTimer.*runCandidate, runControl"
	for range b.N {
		arm()
	}
}

// Fixed-count route evidence: -benchtime=100x -count=7; max/min ceiling.
func BenchmarkRouteSharedHelper(b *testing.B) {
	benchmarkRouteArm(b, runControl)
	benchmarkRouteArm(b, runCandidate)
}

func benchmarkRouteArmRepeated(b *testing.B, arm func()) {
	for range 10 {
		arm()
	}
	b.ResetTimer()
	for range b.N {
		arm()
	}
}

// Fixed-count route evidence: -benchtime=100x -count=7; max/min ceiling.
func BenchmarkRouteSharedHelperRepeated(b *testing.B) {
	benchmarkRouteArmRepeated(b, runControl)
	benchmarkRouteArmRepeated(b, runCandidate)
}

type fakeB struct{}

func (*fakeB) ResetTimer() {}

// A same-named user method is not testing.B.ResetTimer.
func fakeReset(b *fakeB) {
	runControl()
	runCandidate()
	b.ResetTimer()
}
