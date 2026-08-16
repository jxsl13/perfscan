package ps5068

import (
	"fmt"
	"strconv"
)

var _ = fmt.Println
var _ = strconv.Atoi

type buffer []byte

type Celsius float64

// --- POSITIVES ---

func eF64(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%e", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

func fF64(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%f", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

func upperE(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%E", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

func upperG(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%G", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

// float32 is widened to float64(f) with bitSize 32.
func f32(buf []byte, f float32) []byte {
	return fmt.Appendf(buf, "%f", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

// --- ADVISORY: reported, no fix ---

func namedDst(dst buffer, f float64) buffer {
	return fmt.Appendf(dst, "%e", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

func commentInside(buf []byte, f float64) []byte {
	return fmt.Appendf(buf /* keep */, "%e", f) // want `fmt\.Appendf\(buf, "%e"/%f/\.\.\., f\) parses the format and boxes f`
}

// --- NEGATIVES: silent ---

// %g (shortest) is PS5015's.
func shortestG(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%g", f)
}

// A named float type could implement fmt.Formatter.
func namedFloat(buf []byte, c Celsius) []byte {
	return fmt.Appendf(buf, "%f", c)
}

// A width/precision disqualifies the bare verb.
func withPrecision(buf []byte, f float64) []byte {
	return fmt.Appendf(buf, "%.2f", f)
}

// An integer operand is not a float verb here.
func intOperand(buf []byte, n int) []byte {
	return fmt.Appendf(buf, "%f", n)
}
