package ps6010

import "unsafe"

type ps6010Round16NamedSlice []float64
type ps6010Round16NamedArray [8]float64
type ps6010Round16NamedPointer *float64
type ps6010Round16NamedFloat float64

var ps6010Round16GlobalChannel <-chan []float64
var ps6010Round16GlobalSlice []float64
var ps6010Round16GlobalSlices map[int][]float64
var ps6010Round16GlobalPointers map[int]*float64

func ps6010Round16OpaqueSlice() []float64 { return ps6010Round16GlobalSlice }

func ps6010Round16OpaqueSlicePointer() *[]float64 { return &ps6010Round16GlobalSlice }

func ps6010Round16OpaquePointers() map[int]*float64 { return ps6010Round16GlobalPointers }

func ps6010Round16ChannelSource() <-chan []float64 { return ps6010Round16GlobalChannel }

func globalSliceAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		ps6010Round16GlobalSlice[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func opaqueSliceAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias := ps6010Round16OpaqueSlice()
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func opaqueSlicePointerAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias := ps6010Round16OpaqueSlicePointer()
		(*alias)[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func packageMapSliceAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias := ps6010Round16GlobalSlices[0]
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func parameterMapPointerAliasRound16(a []float64, pointers map[int]*float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointers[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func packageMapPointerAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*ps6010Round16GlobalPointers[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func opaqueMapPointerAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		pointer := ps6010Round16OpaquePointers()[0]
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func assertedSliceAliasRound16(a, weights []float64, value any, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias := value.([]float64)
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func rangedPackageMapAliasRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		for _, alias := range ps6010Round16GlobalSlices {
			alias[0] = float64(o)
		}
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func unsafePointerAliasRound16(a, weights []float64, raw unsafe.Pointer, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*(*float64)(raw) = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func uintptrAliasRound16(a, weights []float64, raw uintptr, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*(*float64)(unsafe.Pointer(raw)) = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func namedScalarSlicePointerAliasRound16(a []ps6010Round16NamedFloat, pointer *float64, weights []ps6010Round16NamedFloat, out, n int) [8]ps6010Round16NamedFloat {
	var dst [8]ps6010Round16NamedFloat
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := ps6010Round16NamedFloat(0)
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func namedScalarArrayPointerAliasRound16(a *[8]ps6010Round16NamedFloat, pointer *float64, weights []ps6010Round16NamedFloat, out, n int) [8]ps6010Round16NamedFloat {
	var dst [8]ps6010Round16NamedFloat
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := ps6010Round16NamedFloat(0)
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func globalChannelReceiveRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		func() {
			alias := <-ps6010Round16GlobalChannel
			alias[0] = float64(o)
		}()
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func localOpaqueChannelReceiveRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	channel := ps6010Round16ChannelSource()
	for o := 0; o < out; o++ {
		func() {
			alias := <-channel
			alias[0] = float64(o)
		}()
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func uncapturedReceiveRound16(a, weights []float64, channel <-chan []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		func() {
			alias := <-channel
			alias[0] = float64(o)
		}()
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func nestedNamedReceiveRound16(a ps6010Round16NamedSlice, weights []float64, channel <-chan ps6010Round16NamedSlice, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		func() {
			func() {
				alias := <-channel
				alias[0] = float64(o)
			}()
		}()
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func sameFunctionReceiveRound16(a, weights []float64, channel <-chan []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		alias := <-channel
		alias[0] = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func sliceElementPointerAliasRound16(a []float64, pointer *float64, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func namedArrayElementPointerAliasRound16(a *ps6010Round16NamedArray, pointer ps6010Round16NamedPointer, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o]
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleMapPointerControlRound16(a, weights []float64, pointers map[int]*int, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		*pointers[0] = o
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func incompatibleReceiveControlRound16(a, weights []float64, channel <-chan []int, out, n int) [8]float64 {
	var dst [8]float64
	for o := 0; o < out; o++ {
		func() {
			alias := <-channel
			alias[0] = o
		}()
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func freshPointerControlRound16(a, weights []float64, out, n int) [8]float64 {
	var dst [8]float64
	pointer := new(float64)
	for o := 0; o < out; o++ {
		*pointer = float64(o)
		acc := 0.0
		for i := 0; i < n; i++ {
			acc += a[i] * weights[o] // want `this operand does not vary with the output index o but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load \(bit-identical per output\)`
		}
		dst[o] = acc
	}
	return dst
}

func compileNamedScalarAliasRound16(a []ps6010Round16NamedFloat, array *[8]ps6010Round16NamedFloat) (*float64, *float64) {
	return (*float64)(&a[0]), (*float64)(&array[0])
}
