package benchmarks

import "testing"

const (
	ps6111Elements   = 4096
	ps6111Iterations = 8
)

type ps6111Decoder struct {
	state uint64
}

type ps6111Outcome struct {
	digest      uint64
	finalLogits uint64
	state       uint64
}

//go:noinline
func (decoder *ps6111Decoder) Step(token, position int) ([]float32, error) {
	result := make([]float32, ps6111Elements)
	return result, decoder.StepInto(token, position, result)
}

//go:noinline
func (decoder *ps6111Decoder) StepInto(token, position int, result []float32) error {
	for index := range result {
		result[index] = float32((token+1)*(index+3) + position)
	}
	decoder.state = decoder.state*131 + uint64(token+position+1)
	return nil
}

//go:noinline
func ps6111Before(decoder *ps6111Decoder, seed []float32) (ps6111Outcome, error) {
	ps6111FillSeed(seed)
	current := seed
	var digest uint64
	for step := 0; step < ps6111Iterations; step++ {
		digest = ps6111Consume(current, digest)
		next, err := decoder.Step(step, step+1)
		if err != nil {
			return ps6111Outcome{}, err
		}
		current = next
	}
	return ps6111Outcome{digest: digest, finalLogits: ps6111Consume(current, 0), state: decoder.state}, nil
}

//go:noinline
func ps6111After(decoder *ps6111Decoder, seed, destination []float32) (ps6111Outcome, error) {
	ps6111FillSeed(seed)
	current := seed
	var digest uint64
	for step := 0; step < ps6111Iterations; step++ {
		digest = ps6111Consume(current, digest)
		if err := decoder.StepInto(step, step+1, destination); err != nil {
			return ps6111Outcome{}, err
		}
		current = destination
	}
	return ps6111Outcome{digest: digest, finalLogits: ps6111Consume(current, 0), state: decoder.state}, nil
}

//go:noinline
func ps6111NewDestination(length int) []float32 {
	return make([]float32, length)
}

//go:noinline
func ps6111FillSeed(result []float32) {
	for index := range result {
		result[index] = float32(index%31 - 15)
	}
}

//go:noinline
func ps6111Consume(result []float32, digest uint64) uint64 {
	for _, value := range result {
		digest = digest*1099511628211 ^ uint64(uint32(int32(value*16)))
	}
	return digest
}

var ps6111DigestSink uint64

func TestPS6111WorkPair(t *testing.T) {
	t.Parallel()
	beforeDecoder := new(ps6111Decoder)
	afterDecoder := new(ps6111Decoder)
	before, err := ps6111Before(beforeDecoder, make([]float32, ps6111Elements))
	if err != nil {
		t.Fatal(err)
	}
	after, err := ps6111After(afterDecoder, make([]float32, ps6111Elements), ps6111NewDestination(ps6111Elements))
	if err != nil {
		t.Fatal(err)
	}
	if before != after || before.state != beforeDecoder.state || after.state != afterDecoder.state {
		t.Fatalf("work pair differs: before=%+v/decoder state %d, after=%+v/decoder state %d", before, beforeDecoder.state, after, afterDecoder.state)
	}
}

func TestPS6111WorkPairAllocations(t *testing.T) {
	// testing.AllocsPerRun temporarily sets GOMAXPROCS=1 and rejects parallel
	// tests, so this allocation-accounting test is intentionally serialized.
	beforeDecoder := new(ps6111Decoder)
	afterDecoder := new(ps6111Decoder)
	beforeSeed := make([]float32, ps6111Elements)
	afterSeed := make([]float32, ps6111Elements)
	var before, after ps6111Outcome
	var beforeErr, afterErr error
	beforeAllocations := testing.AllocsPerRun(20, func() {
		beforeDecoder.state = 0
		before, beforeErr = ps6111Before(beforeDecoder, beforeSeed)
	})
	afterAllocations := testing.AllocsPerRun(20, func() {
		afterDecoder.state = 0
		destination := ps6111NewDestination(ps6111Elements)
		after, afterErr = ps6111After(afterDecoder, afterSeed, destination)
	})
	if beforeErr != nil || afterErr != nil {
		t.Fatalf("work pair errors: before=%v after=%v", beforeErr, afterErr)
	}
	if before != after {
		t.Fatalf("allocation work pair differs: before=%+v after=%+v", before, after)
	}
	if beforeAllocations != ps6111Iterations || afterAllocations != 1 {
		t.Fatalf("allocations: before=%g after=%g, want %d and 1", beforeAllocations, afterAllocations, ps6111Iterations)
	}
}

func BenchmarkPS6111_Before(b *testing.B) {
	decoder := new(ps6111Decoder)
	seed := make([]float32, ps6111Elements)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decoder.state = 0
		outcome, err := ps6111Before(decoder, seed)
		if err != nil {
			b.Fatal(err)
		}
		ps6111DigestSink = outcome.digest ^ outcome.finalLogits ^ outcome.state
	}
}

func BenchmarkPS6111_After(b *testing.B) {
	decoder := new(ps6111Decoder)
	seed := make([]float32, ps6111Elements)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		decoder.state = 0
		destination := ps6111NewDestination(ps6111Elements)
		outcome, err := ps6111After(decoder, seed, destination)
		if err != nil {
			b.Fatal(err)
		}
		ps6111DigestSink = outcome.digest ^ outcome.finalLogits ^ outcome.state
	}
}
