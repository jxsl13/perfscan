package ps6005

import (
	"context"
	"os/exec"
	"testing"
)

func BenchmarkLlamaComparison(b *testing.B) {
	cmd := exec.Command("llama-bench", "-m", "tinyllama-q4_k_m.gguf", "-n", "64", "-r", "5") // want `high-confidence comparison claim: external accelerator benchmark pins workload size but leaves recognized semantic axes implicit`
	output, _ := cmd.Output()
	incumbentRatio := float64(len(output)) / 172.2
	_ = incumbentRatio
}

func directGPU(tokens string) *exec.Cmd {
	return exec.Command("gpu-bench", "--tokens", tokens) // want `external accelerator benchmark pins workload size but leaves recognized semantic axes implicit`
}

// Every recognized axis is explicit, so the command is silent. Real harnesses
// must additionally assert the executable's effective-settings manifest.
func explicit() *exec.Cmd {
	return exec.Command("llama-bench",
		"-m", "tinyllama-q4_k_m.gguf",
		"-n", "64",
		"--precision", "f32",
		"-ctk", "f32",
		"-ctv", "f32",
		"--quantization", "q4_k_m",
		"-fa", "0",
		"-ngl", "99",
		"-b", "512",
		"-ub", "128",
		"-c", "2048",
		"--warmup", "1",
		"-r", "5",
	)
}

// No workload-size flag: outside this detector's confidence boundary.
func noWorkload() *exec.Cmd {
	return exec.Command("llama-bench", "-m", "tinyllama-q4_k_m.gguf")
}

// A CPU utility benchmark is not accelerator-related.
func cpuTool() *exec.Cmd {
	return exec.Command("gzip-bench", "--size", "65536")
}

// Opaque variadic arguments may already contain every semantic flag.
func opaque(args []string) *exec.Cmd {
	return exec.Command("llama-bench", args...)
}

// CommandContext is recognized too; this complete form remains silent.
func contextual(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "metal-bench",
		"--tokens", "64", "--dtype", "f32", "--kv-type", "f32",
		"--quant", "q4_k_m", "--flash-attention", "off", "--device", "metal",
		"--batch", "512", "--microbatch", "128", "--context", "2048",
		"--no-warmup", "--runs", "5",
	)
}
