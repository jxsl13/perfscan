// crossover records or independently verifies source-bound diagnostic campaigns.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jxsl13/perfscan/checks"
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/crossover"
)

func ints(text string) ([]int, error) {
	parts := strings.Split(text, ",")
	result := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		result[i] = n
	}
	return result, nil
}

func run() error {
	var o crossover.Options
	var configPath, verify, planPin, recordsPin, sizes, procs string
	var index int
	flag.StringVar(&configPath, "config", ".perfscan.yaml", "repository config with dispatchCrossoverContracts")
	flag.IntVar(&index, "contract", 0, "zero-based contract index")
	flag.StringVar(&o.Repository, "repo", ".", "independently selected source Git repository")
	flag.StringVar(&o.Commit, "commit", "", "exact full source commit identity (recording)")
	flag.StringVar(&o.Output, "out", "", "new retained evidence directory")
	flag.StringVar(&o.Package, "package", ".", "exact package directory relative to source root")
	flag.StringVar(&o.Flags, "goflags", os.Getenv("GOFLAGS"), "controlled GOFLAGS (only -tags= is permitted)")
	flag.StringVar(&o.GoBinary, "go", "go", "independently selected available SDK/bin/go (retained OLD SDK for verification)")
	flag.IntVar(&o.N, "fixed-n", 16, "fixed native work count >=2; 0 enables adaptive duration")
	flag.StringVar(&o.Duration, "duration", "", "positive adaptive duration, only with -fixed-n=0")
	flag.StringVar(&o.Timeout, "timeout", "10m", "predeclared per-process timeout, positive and at most one hour")
	flag.IntVar(&o.Pairs, "pairs", 2, "even fresh-process pairs per complete matrix cell")
	flag.StringVar(&sizes, "sizes", "", "comma-separated extents below and at/above source threshold")
	flag.StringVar(&procs, "procs", "2", "comma-separated distinct GOMAXPROCS >=2")
	flag.StringVar(&verify, "verify", "", "verify retained campaign instead of recording")
	flag.StringVar(&planPin, "plan-sha256", "", "independently retained pre-measurement plan pin")
	flag.StringVar(&recordsPin, "records-sha256", "", "independently retained completed-record pin")
	flag.Parse()
	var report *crossover.Report
	var err error
	if verify != "" {
		report, err = crossover.VerifyCampaign(context.Background(), o.Repository, verify, o.GoBinary, planPin, recordsPin, checks.LoadDispatchCrossoverHarness)
	} else {
		loaded, loadErr := config.Load(configPath)
		if loadErr != nil {
			return loadErr
		}
		if index < 0 || index >= len(loaded.DispatchCrossoverContracts) {
			return errors.New("contract index absent from configuration")
		}
		contract := loaded.DispatchCrossoverContracts[index]
		if contract.Campaign == "" {
			contract.Campaign = o.Output
		}
		o.Sizes, err = ints(sizes)
		if err != nil {
			return err
		}
		o.Procs, err = ints(procs)
		if err != nil {
			return err
		}
		report, err = crossover.RecordCampaign(context.Background(), &o, &contract, checks.LoadDispatchCrossoverHarness, os.Stdout)
	}
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
