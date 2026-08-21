package ps6005alias

import e "os/exec"

func compare() *e.Cmd {
	return e.Command("metal-llama-bench", "--n-predict=64", "--runs=5") // want `external accelerator benchmark pins workload size but leaves recognized semantic axes implicit`
}
