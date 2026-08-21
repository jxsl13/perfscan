package ps6078

func expF64(dst, src []float64) {
	if vexpF64Fast {
		vexpF64NEON(dst, src)
		return
	}
	expF64Scalar(dst, src)
}

func scaledExpF64(dst, src []float64, runtimeReady bool) {
	if vexpF64Fast && runtimeReady {
		vexpF64NEON(dst, src)
		return
	}
	expF64Scalar(dst, src)
}

func equalityExpF64(dst, src []float64) {
	if vexpF64Fast == true {
		vexpF64NEON(dst, src)
		return
	}
	expF64Scalar(dst, src)
}

func softplusF64(dst, src []float64) {
	if !vexpF64Fast {
		expF64Scalar(dst, src)
		return
	}
	softplusF64NEON(dst, src)
}

func helper(dst, src []float64) {
	softplusF64(dst, src)
	scaledExpF64(dst, src, len(src) >= 2)
	equalityExpF64(dst, src)
}

func ExpF64(dst, src []float64) {
	expF64(dst, src)
}

func PublicPipeline(dst, src []float64) {
	helper(dst, src)
}

func vexpF64NEON(dst, src []float64) {
	for index := 0; index+1 < len(src); index += 2 {
		dst[index], dst[index+1] = src[index], src[index+1]
	}
}

func softplusF64NEON(dst, src []float64) {
	for index := 0; index+1 < len(src); index += 2 {
		dst[index], dst[index+1] = src[index], src[index+1]
	}
}

func expF64Scalar(dst, src []float64) {
	copy(dst, src)
}

func equalFlagDoesNotDiffer(dst, src []float64) {
	if equalF32Fast {
		vexpF64NEON(dst, src)
	}
}

func overlapDoesNotDiffer(dst, src []float64) {
	if overlapFast {
		vexpF64NEON(dst, src)
	}
}

func shadowedParameter(vexpF64Fast bool, dst, src []float64) {
	if vexpF64Fast {
		vexpF64NEON(dst, src)
	}
}

func shadowedLocal(dst, src []float64) {
	vexpF64Fast := true
	if vexpF64Fast {
		vexpF64NEON(dst, src)
	}
}

func indirectCall(dst, src []float64) {
	fast := vexpF64NEON
	if vexpF64Fast {
		fast(dst, src)
	}
}

func ordinaryBranch(dst, src []float64) {
	if vexpF64Fast {
		expF64Scalar(dst, src)
	}
}

//perfscan:architecture-capability-validated this route is separately tested.
func validatedRoute(dst, src []float64) {
	if vexpF64Fast {
		vexpF64NEON(dst, src)
	}
}

func closureOnly(dst, src []float64) {
	_ = func() {
		if vexpF64Fast {
			vexpF64NEON(dst, src)
		}
	}
}
