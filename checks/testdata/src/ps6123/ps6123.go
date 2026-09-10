// PS6123's canonical simd/archsimd identity is exercised by the typed pass
// harness in ps6123_test.go. Go 1.27 reserves that standard-library path, so
// the ordinary GOPATH analysistest loader cannot faithfully import its SDK
// fixture.
package ps6123
