package benchmarks

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

type ps6109Shell struct {
	handle uint64
	state  uint64
	freed  bool
}

type ps6109Evidence struct {
	Arm         string
	Allocations float64
	Digest      uint64
	LastHandle  uint64
}

type ps6109StepEvidence struct {
	Digest     uint64
	Generation uint64
	Empty      bool
	Idempotent bool
}

var ps6109EscapingShell *ps6109Shell
var ps6109Observable uint64

func ps6109FreshShell(next *uint64) *ps6109Shell {
	shell := &ps6109Shell{}
	shell.reset(next)
	ps6109EscapingShell = shell
	return shell
}

//go:noinline
func (shell *ps6109Shell) reset(next *uint64) {
	*next++
	shell.handle = *next
	shell.state = 0
	shell.freed = false
}

//go:noinline
func (shell *ps6109Shell) use(value uint64) { shell.state = shell.handle*17 + value }

//go:noinline
func (shell *ps6109Shell) free() {
	if shell.freed {
		return
	}
	shell.handle = 0
	shell.state = 0
	shell.freed = true
}

//go:noinline
func ps6109BeforeStep(next *uint64, value uint64) ps6109StepEvidence {
	shell := ps6109FreshShell(next)
	shell.use(value)
	evidence := ps6109StepEvidence{Digest: shell.state, Generation: shell.handle}
	shell.free()
	evidence.Empty = shell.handle == 0 && shell.state == 0 && shell.freed
	shell.free()
	evidence.Idempotent = shell.handle == 0 && shell.state == 0 && shell.freed
	return evidence
}

//go:noinline
func ps6109AfterStep(shell *ps6109Shell, next *uint64, value uint64) ps6109StepEvidence {
	shell.reset(next)
	shell.use(value)
	evidence := ps6109StepEvidence{Digest: shell.state, Generation: shell.handle}
	shell.free()
	evidence.Empty = shell.handle == 0 && shell.state == 0 && shell.freed
	shell.free()
	evidence.Idempotent = shell.handle == 0 && shell.state == 0 && shell.freed
	return evidence
}

func BenchmarkPS6109_Before(b *testing.B) {
	var next, digest uint64
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		digest += ps6109BeforeStep(&next, uint64(index)).Digest
	}
	ps6109Observable = digest + next
}

func BenchmarkPS6109_After(b *testing.B) {
	var shell ps6109Shell
	var next, digest uint64
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		digest += ps6109AfterStep(&shell, &next, uint64(index)).Digest
	}
	ps6109Observable = digest + next
}

func TestPS6109WorkPair(t *testing.T) {
	t.Parallel()
	results := make(chan ps6109Evidence, 2)
	for _, arm := range []string{"before", "after"} {
		arm := arm
		t.Run(arm, func(t *testing.T) {
			t.Parallel()
			command := exec.Command(os.Args[0], "-test.run=^TestPS6109WorkPairWorker$", "-test.count=1")
			command.Env = append(os.Environ(), "PERFSCAN_PS6109_ARM="+arm)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("isolated %s worker: %v", arm, err)
			}
			var evidence ps6109Evidence
			line, _, _ := bytes.Cut(output, []byte{'\n'})
			if err := json.Unmarshal(line, &evidence); err != nil {
				t.Fatalf("decode isolated %s evidence %q: %v", arm, output, err)
			}
			results <- evidence
		})
	}
	t.Cleanup(func() {
		close(results)
		var before, after ps6109Evidence
		for result := range results {
			if result.Arm == "before" {
				before = result
			} else {
				after = result
			}
		}
		if before.Allocations != 1 || after.Allocations != 0 {
			t.Errorf("allocations before/after = %.0f/%.0f, want 1/0", before.Allocations, after.Allocations)
		}
		if before.Digest != after.Digest || before.LastHandle != after.LastHandle || before.LastHandle != 32 {
			t.Errorf("work mismatch: before=%+v after=%+v", before, after)
		}
	})
}

func TestPS6109WorkPairWorker(t *testing.T) {
	// Intentionally serial: testing.AllocsPerRun rejects parallel tests, and the
	// parent already isolates each arm in its own process.
	arm := os.Getenv("PERFSCAN_PS6109_ARM")
	if arm == "" {
		t.Skip("isolated worker")
	}
	var shell ps6109Shell
	var allocationNext uint64
	allocations := testing.AllocsPerRun(100, func() {
		if arm == "before" {
			ps6109Observable = ps6109BeforeStep(&allocationNext, 1).Digest
		} else {
			ps6109Observable = ps6109AfterStep(&shell, &allocationNext, 1).Digest
		}
	})
	var next, digest uint64
	var priorGeneration uint64
	for index := 0; index < 32; index++ {
		var step ps6109StepEvidence
		if arm == "before" {
			step = ps6109BeforeStep(&next, uint64(index))
		} else if arm == "after" {
			step = ps6109AfterStep(&shell, &next, uint64(index))
		} else {
			t.Fatalf("unknown arm %q", arm)
		}
		if step.Generation <= priorGeneration || !step.Empty || !step.Idempotent {
			t.Fatalf("generation %d after %d: %+v", step.Generation, priorGeneration, step)
		}
		priorGeneration = step.Generation
		digest += step.Digest
	}
	if ps6109EscapingShell != nil && arm == "before" && !ps6109EscapingShell.freed {
		t.Fatal("terminal did not clear final before shell")
	}
	encoded, err := json.Marshal(ps6109Evidence{Arm: arm, Allocations: allocations, Digest: digest, LastHandle: next})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = os.Stdout.Write(append(encoded, '\n'))
}

func TestPS6109TwoOverlappingShells(t *testing.T) {
	t.Parallel()
	var next uint64
	first, second := ps6109Shell{}, ps6109Shell{}
	first.reset(&next)
	second.reset(&next)
	if first.handle == 0 || second.handle == 0 || first.handle == second.handle {
		t.Fatalf("overlapping generations are not distinct: first=%+v second=%+v", first, second)
	}
	first.use(3)
	second.use(5)
	if first.state == second.state {
		t.Fatal("overlapping shell state aliased")
	}
	second.free()
	first.free()
	if !first.freed || !second.freed || first.handle != 0 || second.handle != 0 {
		t.Fatalf("overlapping terminal state not empty: first=%+v second=%+v", first, second)
	}
}

// resetFallible models the contract's required empty state on reset failure.
//
//go:noinline
func (shell *ps6109Shell) resetFallible(next *uint64, fail bool) bool {
	shell.free()
	if fail {
		return false
	}
	shell.reset(next)
	return true
}

func TestPS6109ResetFailurePreservesAllocatingFallback(t *testing.T) {
	t.Parallel()
	var next uint64
	shell := &ps6109Shell{}
	shell.reset(&next)
	if shell.resetFallible(&next, true) || shell.handle != 0 || shell.state != 0 || !shell.freed {
		t.Fatalf("failed reset retained generation state: %+v", shell)
	}
	fallback := ps6109FreshShell(&next)
	if fallback == shell || fallback.handle == 0 || fallback.freed {
		t.Fatalf("allocating fallback did not create a fresh shell: old=%p new=%+v", shell, fallback)
	}
	fallback.free()
}
