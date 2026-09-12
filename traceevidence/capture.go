// Package traceevidence retains and validates xctrace profiling captures.
// Acceptance establishes readable artifacts and workload markers, not a
// performance improvement, uncontaminated counters, or target process exit 0.
package traceevidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Schema selects one required table and its required column mnemonics. These
// names must come from the selected Instruments version and capture policy.
// They are not inferred from instrument display names.
type Schema struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

type Options struct {
	Xcrun              string
	Output             string
	Directory          string
	Workload           []string
	Instruments        []string
	Schemas            []Schema
	Started, Completed string
	TimeLimit          time.Duration
	CommandTimeout     time.Duration
	MaxArtifactBytes   int64
	MaxTraceBytes      int64
	InputPolicy        *InputPolicy
}

// Invocation describes an actually executed command. A start, cancellation,
// export, stream-write, or deadline failure is never a successful invocation.
type Invocation struct {
	Arguments []string `json:"arguments"`
	Exit      int      `json:"exit"`
	Failure   string   `json:"failure"`
}

type Result struct {
	Accepted       bool                  `json:"accepted"`
	TimeLimited    bool                  `json:"timeLimited"`
	TraceSHA256    string                `json:"traceSHA256"`
	TableRows      map[string]int64      `json:"tableRows"`
	Invocations    map[string]Invocation `json:"invocations"`
	Reason         string                `json:"reason"`
	InputPreflight *InputPreflight       `json:"inputPreflight,omitempty"`
}

// Capture creates a NEW evidence directory. It retains every command's raw
// streams/status and all partial trace/target/export artifacts on failure.
// It never retries a rejected sample, removes captures, stages protected
// inputs, or changes privacy permissions. Callers must independently qualify
// target input access and the meaning of their workload completion marker.
func Capture(ctx context.Context, o *Options) (*Result, error) {
	return capture(ctx, o, nil)
}

type commandRunner func(context.Context, string, string, string, []string, int64) Invocation

func capture(ctx context.Context, o *Options, runner commandRunner) (*Result, error) {
	return captureInInputEnvironment(ctx, o, runner, nil)
}

func captureInInputEnvironment(ctx context.Context, o *Options, runner commandRunner, environment *inputEnvironment) (*Result, error) {
	if ctx == nil || o == nil {
		return nil, errors.New("capture requires context and options")
	}
	if err := validOptions(o); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(o.Output)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return nil, fmt.Errorf("new capture directory: %w", err)
	}
	result := &Result{Invocations: make(map[string]Invocation), TableRows: make(map[string]int64)}
	finish := func(problem error) (*Result, error) {
		if problem != nil {
			result.Reason = problem.Error()
		}
		data, encodeErr := json.MarshalIndent(result, "", "  ")
		if encodeErr == nil {
			encodeErr = writeNew(filepath.Join(root, "result.json"), append(data, '\n'))
		}
		if encodeErr != nil {
			result.Accepted = false
			result.Reason = errors.Join(problem, encodeErr).Error()
		}
		return result, errors.Join(problem, encodeErr)
	}
	plan, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return finish(err)
	}
	if err := writeNew(filepath.Join(root, "plan.json"), append(plan, '\n')); err != nil {
		return finish(err)
	}
	preflightContext, cancelPreflight := context.WithTimeout(ctx, o.CommandTimeout)
	if environment == nil {
		result.InputPreflight, err = PreflightInputs(preflightContext, o.Directory, o.Workload, o.InputPolicy)
	} else {
		result.InputPreflight, err = preflightInputs(preflightContext, o.Directory, o.Workload, o.InputPolicy, *environment)
	}
	cancelPreflight()
	preflightData, encodePreflightErr := json.MarshalIndent(result.InputPreflight, "", "  ")
	if encodePreflightErr == nil {
		encodePreflightErr = writeNew(filepath.Join(root, "input-preflight.json"), append(preflightData, '\n'))
	}
	if err != nil || encodePreflightErr != nil {
		return finish(errors.Join(err, encodePreflightErr))
	}
	workload := slices.Clone(o.Workload)
	workload[0] = result.InputPreflight.Inputs[0].Canonical
	executable := o.Xcrun
	if executable == "" {
		executable = "xcrun"
	}
	if runner == nil {
		runner = runCommand
	}
	run := func(name string, args ...string) Invocation {
		bounded, cancel := context.WithTimeout(ctx, o.CommandTimeout)
		defer cancel()
		invocation := runner(bounded, executable, o.Directory, filepath.Join(root, name), args, o.MaxArtifactBytes)
		result.Invocations[name] = invocation
		return invocation
	}
	version := run("version", "xctrace", "version")
	if version.Exit != 0 || version.Failure != "" {
		return finish(errors.New("cannot observe xctrace version; raw diagnostics retained"))
	}
	versionText, err := readRegular(filepath.Join(root, "version.stdout"), o.MaxArtifactBytes)
	if err != nil || strings.TrimSpace(string(versionText)) == "" {
		return finish(errors.New("missing observed xctrace version output"))
	}
	trace := filepath.Join(root, "capture.trace")
	target := filepath.Join(root, "target.txt")
	args := []string{"xctrace", "record"}
	for _, instrument := range o.Instruments {
		args = append(args, "--instrument", instrument)
	}
	args = append(args, "--time-limit", strconv.FormatInt(o.TimeLimit.Milliseconds(), 10)+"ms", "--no-prompt", "--output", trace, "--target-stdout", target, "--launch", "--")
	args = append(args, workload...)
	record := run("record", args...)
	if record.Failure != "" || record.Exit != 0 && record.Exit != 54 {
		return finish(errors.New("recorder failed outside the qualified 0/54 outcomes; capture retained"))
	}
	if record.Exit == 54 {
		stdout, err := readRegular(filepath.Join(root, "record.stdout"), o.MaxArtifactBytes)
		if err != nil {
			return finish(err)
		}
		stderr, err := readRegular(filepath.Join(root, "record.stderr"), o.MaxArtifactBytes)
		if err != nil {
			return finish(err)
		}
		if !timeoutMarkers(string(stdout)+"\n"+string(stderr), trace) {
			return finish(errors.New("exit 54 without the known time-limit/completed/exact-output markers"))
		}
	}
	before, err := bundleDigestContext(ctx, trace, o.MaxTraceBytes)
	if err != nil {
		return finish(err)
	}
	output, err := readRegular(target, o.MaxArtifactBytes)
	if err != nil {
		return finish(err)
	}
	if !workloadInputMarkers(output, o.Started, o.InputPolicy.Opened, o.Completed) {
		return finish(errors.New("missing, duplicated or unordered workload start/completion markers"))
	}
	if invocation := run("toc", "xctrace", "export", "--input", trace, "--toc"); invocation.Exit != 0 || invocation.Failure != "" {
		return finish(errors.New("TOC export failed; raw diagnostics retained"))
	}
	toc, err := readRegular(filepath.Join(root, "toc.stdout"), o.MaxArtifactBytes)
	if err != nil {
		return finish(err)
	}
	paths, err := tablePaths(toc, o.Schemas)
	if err != nil {
		return finish(err)
	}
	for i, schema := range o.Schemas {
		name := "table-" + strconv.Itoa(i)
		xpath := "/trace-toc/run[@number=\"1\"]/data/table[@schema=\"" + schema.Name + "\"]"
		if invocation := run(name, "xctrace", "export", "--input", trace, "--xpath", xpath); invocation.Exit != 0 || invocation.Failure != "" {
			return finish(fmt.Errorf("required table %s export failed", schema.Name))
		}
		data, err := readRegular(filepath.Join(root, name+".stdout"), o.MaxArtifactBytes)
		if err != nil {
			return finish(err)
		}
		rows, err := validateTable(data, xpath, schema, paths[schema.Name])
		if err != nil {
			return finish(err)
		}
		result.TableRows[schema.Name] = rows
	}
	after, err := bundleDigestContext(ctx, trace, o.MaxTraceBytes)
	if err != nil || before != after {
		return finish(errors.New("trace bundle changed during exports"))
	}
	current, err := readRegular(target, o.MaxArtifactBytes)
	if err != nil || string(current) != string(output) {
		return finish(errors.New("target output changed during validation"))
	}
	if err := ctx.Err(); err != nil {
		return finish(err)
	}
	result.Accepted, result.TimeLimited, result.TraceSHA256 = true, record.Exit == 54, before
	return finish(nil)
}

func validOptions(o *Options) error {
	if !o.InputPolicy.valid() || o.InputPolicy.Opened == o.Started || o.InputPolicy.Opened == o.Completed {
		return errors.New("explicit audited-complete input inventory and distinct post-input-open progress marker are required")
	}
	if o.Output == "" || len(o.Workload) == 0 || o.Workload[0] == "" || len(o.Instruments) == 0 || len(o.Schemas) == 0 || len(o.Schemas) > 32 || o.TimeLimit < time.Millisecond || o.CommandTimeout <= o.TimeLimit || o.CommandTimeout > 24*time.Hour || o.MaxArtifactBytes < 1 || o.MaxArtifactBytes > 64<<20 || o.MaxTraceBytes < 1 || o.MaxTraceBytes > 8<<30 {
		return errors.New("capture needs a new output path, workload, instruments, required schemas, positive byte limits and timeout longer than time-limit")
	}
	if o.Started == o.Completed || !singleLine(o.Started) || !singleLine(o.Completed) {
		return errors.New("distinct exact single-line workload markers are required")
	}
	names := make([]string, 0, len(o.Schemas))
	for _, schema := range o.Schemas {
		if !schemaName(schema.Name) || len(schema.Columns) == 0 || slices.Contains(names, schema.Name) {
			return errors.New("distinct required table schemas and required column mnemonics are mandatory")
		}
		names = append(names, schema.Name)
		for i, column := range schema.Columns {
			if !schemaName(column) || slices.Contains(schema.Columns[:i], column) {
				return errors.New("invalid or duplicated required column mnemonic")
			}
		}
	}
	for _, instrument := range o.Instruments {
		if !singleLine(instrument) {
			return errors.New("empty or multiline instrument")
		}
	}
	return nil
}

func singleLine(s string) bool { return s != "" && !strings.ContainsAny(s, "\r\n\x00") }

func schemaName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '-' && r != '_' && r != '.' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return errors.Join(err, f.Close())
}

type cappedWriter struct {
	writer io.Writer
	left   int64
	cancel context.CancelFunc
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.left {
		n, err := w.writer.Write(data[:w.left])
		w.left = 0
		w.cancel()
		return n, errors.Join(errors.New("raw command output exceeds byte limit"), err)
	}
	n, err := w.writer.Write(data)
	w.left -= int64(n)
	if err != nil {
		w.cancel()
	}
	return n, err
}

func runCommand(ctx context.Context, executable, directory, prefix string, args []string, limit int64) Invocation {
	result := Invocation{Arguments: slices.Clone(args), Exit: -1}
	stdout, err := os.OpenFile(prefix+".stdout", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		result.Failure = err.Error()
		return result
	}
	stderr, err := os.OpenFile(prefix+".stderr", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		result.Failure = errors.Join(err, stdout.Close()).Error()
		return result
	}
	bounded, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(bounded, executable, args...)
	cmd.Dir = directory
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout = &cappedWriter{stdout, limit, cancel}
	cmd.Stderr = &cappedWriter{stderr, limit, cancel}
	err = cmd.Run()
	if cmd.ProcessState != nil {
		result.Exit = cmd.ProcessState.ExitCode()
	}
	var exit *exec.ExitError
	// A recorder's ordinary nonzero exit is classified only after artifacts.
	// All other failures, including output limits and cancellation, are fatal.
	if errors.As(err, &exit) && bounded.Err() == nil {
		err = nil
	}
	err = errors.Join(err, bounded.Err(), stdout.Close(), stderr.Close())
	if err != nil {
		result.Failure = err.Error()
	}
	data, err := json.Marshal(result)
	if err == nil {
		err = writeNew(prefix+".status.json", append(data, '\n'))
	}
	if err != nil {
		result.Failure = errors.Join(errors.New(result.Failure), err).Error()
	}
	return result
}
