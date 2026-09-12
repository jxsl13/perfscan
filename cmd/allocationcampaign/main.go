// allocationcampaign builds, runs and verifies retained exact allocation
// campaigns. Run -help for options; verification can be rerun independently.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jxsl13/perfscan/internal/allocationcampaign"
)

func main() {
	var options allocationcampaign.Options
	var verify, procs, planHash, recordHash, implementation string
	flag.StringVar(&options.OldRoot, "old", "", "clean pinned old source tree")
	flag.StringVar(&options.CandidateRoot, "candidate", "", "clean pinned candidate source tree")
	flag.StringVar(&options.Output, "out", "", "new evidence directory")
	flag.StringVar(&options.Diagnostic, "diagnostic", "benchmarks/allocation_diagnostic_test.go", "identical diagnostic file relative to each root")
	flag.StringVar(&options.Package, "package", "./benchmarks", "diagnostic package relative to each root")
	flag.StringVar(&implementation, "instrumentation", "", "additional comma-separated diagnostic implementation files/directories to match (wrapper and benchmarkevidence are always included)")
	flag.IntVar(&options.Pairs, "pairs", 2, "even number of pairs per phase/process count")
	flag.StringVar(&procs, "procs", "1,2", "comma-separated positive GOMAXPROCS values")
	flag.StringVar(&verify, "verify", "", "independently verify existing evidence directory instead of running")
	flag.StringVar(&planHash, "plan-sha256", "", "independently retained pre-measurement plan SHA256 for verification")
	flag.StringVar(&recordHash, "records-sha256", "", "independently retained post-measurement records SHA256 for verification")
	flag.Parse()
	if implementation != "" {
		options.Instrumentation = strings.Split(implementation, ",")
	}
	var err error
	if verify != "" {
		pairs, verifyErr := allocationcampaign.Verify(verify, planHash, recordHash)
		err = verifyErr
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(pairs)
		}
	} else {
		for value := range strings.SplitSeq(procs, ",") {
			p, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				err = parseErr
				break
			}
			options.Procs = append(options.Procs, p)
		}
		if err == nil {
			err = allocationcampaign.Run(&options)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
