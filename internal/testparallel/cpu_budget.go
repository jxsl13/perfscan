package main

import (
	"strconv"
	"strings"
)

// Multiple Go processes each see the same host CPU quota. Dividing that quota
// prevents their defaults (including nested Go builds) from multiplying it.
// At least one execution slot remains when workers exceed available CPUs.
func workerCPUShare(available, workers int) int {
	return max(1, max(1, available)/max(1, workers))
}

func workerEnvironment(environment []string, procs int, goos string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		// Windows environment names are case-insensitive. Never leave another
		// spelling that can override the source-selected process budget.
		if name != "GOMAXPROCS" && !(goos == "windows" && strings.EqualFold(name, "GOMAXPROCS")) {
			result = append(result, entry)
		}
	}
	return append(result, "GOMAXPROCS="+strconv.Itoa(max(1, procs)))
}
