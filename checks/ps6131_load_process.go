package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/allocationcampaign"
	"github.com/jxsl13/perfscan/internal/crossover"
)

const ps6131LoaderArgument = "--perfscan-internal-ps6131-typed-loader"

type ps6131LoadRequest struct {
	Selection         crossover.BuildSelection
	Contract          config.DispatchCrossoverContract
	DeadlineUnixNanos int64
}

type ps6131LoadResponse struct {
	Model crossover.HarnessModel
	Error string
	SDK   string
}

// x/tools resolves exec.Command("go") against the loading PROCESS's PATH,
// not packages.Config.Env. Re-execution isolates SDK selection without changing
// global environment during concurrent scans. Only our own executable receives
// this private entry argument; evidence cannot supply an executable or model.
func init() {
	if len(os.Args) != 2 || os.Args[1] != ps6131LoaderArgument {
		return
	}
	var request ps6131LoadRequest
	response := ps6131LoadResponse{}
	data, readErr := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if readErr != nil || len(data) > 1<<20 {
		response.Error = "typed loader request exceeds limit or cannot be read"
	} else if err := allocationcampaign.Decode(data, &request); err != nil {
		response.Error = err.Error()
	} else if deadline := time.Unix(0, request.DeadlineUnixNanos); !deadline.After(time.Now()) || deadline.After(time.Now().Add(10*time.Minute)) {
		response.Error = "typed loader deadline is expired or exceeds limit"
	} else {
		ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, request.DeadlineUnixNanos))
		defer cancel()
		request.Selection.Context = ctx
		_, sdk, err := request.Selection.Environment(ctx)
		response.SDK = sdk
		var model *crossover.HarnessModel
		if err == nil {
			model, err = ps6131LoadLocal(&request.Selection, &request.Contract)
		}
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Model = *model
		}
	}
	ps6131NormalizeModel(&response.Model)
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func ps6131LoadSubprocess(selection *crossover.BuildSelection, contract *config.DispatchCrossoverContract) (*crossover.HarnessModel, error) {
	if selection == nil || contract == nil {
		return nil, errors.New("missing controlled SDK selection or dispatch contract")
	}
	parent := context.Background()
	if selection.Context != nil {
		parent = selection.Context
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	env, binary, err := selection.Environment(ctx)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	copySelection := *selection
	copySelection.Context = nil
	copySelection.GoBinary = binary
	copyContract := *contract
	if copyContract.LeafKernels == nil {
		copyContract.LeafKernels = []string{}
	}
	deadline, _ := ctx.Deadline()
	request, err := json.Marshal(ps6131LoadRequest{Selection: copySelection, Contract: copyContract, DeadlineUnixNanos: deadline.UnixNano()})
	if err != nil {
		return nil, err
	}
	if len(request) > 1<<20 {
		return nil, errors.New("typed loader request exceeds limit")
	}
	command := exec.CommandContext(ctx, executable, ps6131LoaderArgument)
	command.Env = env
	command.Dir = selection.Root
	command.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	command.Stdout = &ps6131LimitedWriter{Buffer: &stdout, Remaining: 64 << 20}
	command.Stderr = &ps6131LimitedWriter{Buffer: &stderr, Remaining: 1 << 20}
	command.WaitDelay = 5 * time.Second
	if err := command.Run(); err != nil {
		return nil, errors.New("controlled typed-loader process failed: " + err.Error() + ": " + stderr.String())
	}
	var response ps6131LoadResponse
	if err := allocationcampaign.Decode(stdout.Bytes(), &response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	if response.SDK != binary || response.Model.PackagePath == "" {
		return nil, errors.New("controlled typed loader returned a different SDK or no observed model")
	}
	return &response.Model, nil
}

type ps6131LimitedWriter struct {
	Buffer    *bytes.Buffer
	Remaining int
}

func (w *ps6131LimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.Remaining {
		return 0, errors.New("typed loader output exceeds limit")
	}
	w.Remaining -= len(data)
	return w.Buffer.Write(data)
}

func ps6131NormalizeModel(m *crossover.HarnessModel) {
	if m.Imports == nil {
		m.Imports = map[string]string{}
	}
	if m.SourceSHA256 == nil {
		m.SourceSHA256 = map[string]string{}
	}
	if m.TypedFileSHA256 == nil {
		m.TypedFileSHA256 = map[string]string{}
	}
	if m.TypedPackageFiles == nil {
		m.TypedPackageFiles = map[string][]string{}
	}
	if m.OriginalSizes == nil {
		m.OriginalSizes = []int{}
	}
	if m.ErrorResults == nil {
		m.ErrorResults = []int{}
	}
}
