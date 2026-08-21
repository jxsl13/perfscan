package ps6073

import "runtime"

type Float interface {
	~float32 | ~float64
}

type OnlyF32 interface {
	~float32
}

type NoF32 interface {
	~int | ~float64
}

type MethodFloat interface {
	~float32 | ~float64
	Value() float64
}

const useF32SIMD = true
const ordinaryFlag = true

var useNativeF32 = runtime.GOARCH == "arm64"

func rmsNormFwd[T Float](values []T) { // want `rmsNormFwd guards a runtime assertion to \[\]float32 with package flag useF32SIMD even though its generic constraint also admits float64`
	if useF32SIMD {
		if values32, ok := any(values).([]float32); ok {
			_ = values32
		}
	}
}

func layerNormForward[T Float](values []T) { // want `layerNormForward guards a runtime assertion to \[\]float32 with package flag enabledByBuild even though its generic constraint also admits float64`
	if enabledByBuild {
		switch values32 := any(values).(type) {
		case []float32:
			_ = values32
		default:
		}
	}
}

func tensorKernel[T Float](values []T) { // want `tensorKernel guards a runtime assertion to \[\]float32 with package flag useNativeF32 even though its generic constraint also admits float64`
	if useNativeF32 && len(values) != 0 {
		_, _ = any(values).([]float32)
	}
}

// A neutral name is hot only because its direct call is in a repeated loop region.
func dispatch[T Float](values []T) { // want `dispatch guards a runtime assertion to \[\]float32 with package flag useF32SIMD even though its generic constraint also admits float64`
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

func drive(rows [][]float64) {
	for _, row := range rows {
		dispatch(row)
	}
}

type Buffer[T Float] struct {
	values []T
}

func (buffer *Buffer[T]) reduceVector() { // want `reduceVector guards a runtime assertion to \[\]float32 with package flag useF32SIMD even though its generic constraint also admits float64`
	if useF32SIMD {
		_, _ = any(buffer.values).([]float32)
	}
}

// Unrestricted parameters also have non-target instantiations.
func softmaxAny[T any](values []T) { // want `softmaxAny guards a runtime assertion to \[\]float32 with package flag useF32SIMD even though its generic constraint also admits other dtypes`
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

// A one-dtype constraint has no non-target instantiation.
func oneDtypeNorm[T OnlyF32](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

// The asserted built-in dtype must itself be admitted by the constraint.
func excludedDtypeNorm[T NoF32](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

// Explicit methods exclude the asserted built-in dtype.
func methodConstraintNorm[T MethodFloat](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

// The asserted shape must be an actual instantiation of the generic source.
func mismatchedShapeNorm[T Float](values []T) {
	if useF32SIMD {
		_, _ = any(values).(*float32)
	}
}

func mismatchedArrayNorm[T Float](values [2]T) {
	if useF32SIMD {
		_, _ = any(values).([3]float32)
	}
}

// An ordinary package bool without specialization provenance stays silent.
func ordinaryNorm[T Float](values []T) {
	if ordinaryFlag {
		_, _ = any(values).([]float32)
	}
}

// Local flags do not establish a build/architecture specialization.
func localNorm[T Float](values []T, enabled bool) {
	if enabled {
		_, _ = any(values).([]float32)
	}
}

// The assertion must consume generic data.
func concreteNorm[T Float](values []T, concrete []float32) {
	_ = values
	if useF32SIMD {
		_, _ = any(concrete).([]float32)
	}
}

// The assertion must be guarded by the specialization flag.
func unguardedNorm[T Float](values []T) {
	_, _ = any(values).([]float32)
}

// A nonnumeric target is unrelated to dtype-specialized numeric kernels.
func stringNorm[T Float](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]string)
	}
}

// A cold generic helper with a neutral name and no repeated caller stays silent.
func choose[T Float](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

// Calls evaluated once do not establish hotness.
func initChoice[T Float](values []T) []int {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
	return nil
}

func once(rows [][]float64) {
	for i := initChoice(rows[0]); i == nil; {
		break
	}
}

func rangeChoice[T Float](values []T) []int {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
	return nil
}

func rangeOnce(values []float64) {
	for range rangeChoice(values) {
	}
}

func capturedChoice[T Float](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}

func capture(rows [][]float64) []func() {
	callbacks := make([]func(), 0, len(rows))
	for _, row := range rows {
		callbacks = append(callbacks, func() { capturedChoice(row) })
	}
	return callbacks
}

// A nested closure owns its own execution context and generic capture.
func closureNorm[T Float](values []T) func() {
	if useF32SIMD {
		return func() {
			_, _ = any(values).([]float32)
		}
	}
	return func() {}
}

//perfscan:generic-fastpath-layout-validated objdump layout compared.
func validatedNorm[T Float](values []T) {
	if useF32SIMD {
		_, _ = any(values).([]float32)
	}
}
