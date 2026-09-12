package traceevidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/macho"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

//go:embed supervisor/supervisor.c
var supervisorSource []byte

// Supervision describes observed direct-child ownership, never descendant or
// service containment. Scope acknowledgement is not proof of process topology.
type Supervision struct {
	Protocol       int    `json:"protocol"`
	TargetPID      int    `json:"targetPID"`
	RecorderPID    int    `json:"recorderPID"`
	Notification   string `json:"notification"`
	Ready          bool   `json:"ready"`
	Resumed        bool   `json:"resumed"`
	TargetReaped   bool   `json:"targetReaped"`
	RecorderReaped bool   `json:"recorderReaped"`
	TargetStatus   int    `json:"targetStatus"`
	RecorderStatus int    `json:"recorderStatus"`
	Forced         bool   `json:"forced"`
	Canceled       bool   `json:"canceled"`
	Failed         bool   `json:"failed"`
}

func (s *Supervision) valid() bool {
	return s.Protocol == 1 && s.TargetPID > 0 && s.TargetPID <= 2147483647 && s.RecorderPID > 0 && s.RecorderPID <= 2147483647 && s.RecorderPID != s.TargetPID && strings.HasPrefix(s.Notification, "org.perfscan.supervisor.") && singleLine(s.Notification) && s.Ready && s.Resumed && s.TargetReaped && s.RecorderReaped && s.TargetStatus == 0 && (s.RecorderStatus == 0 || s.RecorderStatus == 54<<8) && !s.Forced && !s.Canceled && !s.Failed
}

func decodeSupervision(data []byte) (*Supervision, error) {
	if len(data) > 4096 {
		return nil, errors.New("supervisor protocol exceeds limit")
	}
	// Reject duplicate fields as well as unknown fields and trailing responses.
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, errors.New("invalid supervisor object")
	}
	seen := map[string]bool{}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		name, ok := t.(string)
		if !ok || seen[name] {
			return nil, errors.New("duplicate supervisor field")
		}
		switch name {
		case "protocol", "targetPID", "recorderPID", "notification", "ready", "resumed", "targetReaped", "recorderReaped", "targetStatus", "recorderStatus", "forced", "canceled", "failed":
		default:
			return nil, errors.New("noncanonical supervisor field")
		}
		seen[name] = true
		var v json.RawMessage
		if err = d.Decode(&v); err != nil {
			return nil, err
		}
		if string(bytes.TrimSpace(v)) == "null" {
			return nil, errors.New("null supervisor field")
		}
	}
	if _, err = d.Token(); err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errors.New("trailing supervisor response")
	}
	if len(seen) != 13 {
		return nil, errors.New("incomplete supervisor response")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var s Supervision
	if err = d.Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func fileSHA(ctx context.Context, path string, limit int64) (string, error) {
	digest, _, err := digestRegular(ctx, path, limit)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest), nil
}

func prepareSupervisor(ctx context.Context, o *Options, root string) (string, error) {
	return prepareSupervisorWithRunner(ctx, o, root, runCommand, runtime.GOOS, runtime.GOARCH)
}

func prepareSupervisorWithRunner(ctx context.Context, o *Options, root string, runner commandRunner, platform, architecture string) (string, error) {
	if platform != "darwin" || architecture != "arm64" && architecture != "amd64" {
		return "", errors.New("native supervised attach requires Darwin arm64 or amd64")
	}
	source := filepath.Join(root, "supervisor.c")
	binary := filepath.Join(root, "supervisor")
	if err := writeNew(source, supervisorSource); err != nil {
		return "", err
	}
	observations := map[string]string{"sourceSHA256": fmt.Sprintf("%x", sha256.Sum256(supervisorSource)), "protocol": "1", "architecture": architecture}
	observe := func(name, executable string, args ...string) (string, error) {
		bounded, cancel := context.WithTimeout(ctx, o.CommandTimeout)
		defer cancel()
		prefix := filepath.Join(root, "supervisor-"+name)
		inv := runner(bounded, executable, "", prefix, args, o.MaxArtifactBytes)
		if inv.Exit != 0 || inv.Failure != "" {
			return "", fmt.Errorf("supervisor %s observation failed: %s", name, inv.Failure)
		}
		data, err := readRegular(prefix+".stdout", o.MaxArtifactBytes)
		if err != nil {
			return "", err
		}
		text := strings.TrimSpace(string(data))
		if name != "compile" && text == "" {
			return "", errors.New("missing observed native compiler/SDK identity")
		}
		observations[name] = text
		return text, nil
	}
	compiler, err := observe("compiler-path", "/usr/bin/xcrun", "--find", "clang")
	if err != nil {
		return "", err
	}
	sdk, err := observe("sdk-path", "/usr/bin/xcrun", "--sdk", "macosx", "--show-sdk-path")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(compiler) || !filepath.IsAbs(sdk) || strings.ContainsAny(compiler+sdk, "\r\n\x00") {
		return "", errors.New("invalid observed compiler or SDK path")
	}
	compiler, err = filepath.EvalSymlinks(compiler)
	if err != nil {
		return "", err
	}
	sdk, err = filepath.EvalSymlinks(sdk)
	if err != nil {
		return "", err
	}
	observations["canonicalCompiler"], observations["canonicalSDK"] = compiler, sdk
	if _, err = observe("developer-directory", "/usr/bin/xcode-select", "-p"); err != nil {
		return "", err
	}
	if _, err = observe("sdk-version", "/usr/bin/xcrun", "--sdk", "macosx", "--show-sdk-version"); err != nil {
		return "", err
	}
	if _, err = observe("compiler-version", compiler, "--version"); err != nil {
		return "", err
	}
	observations["compilerSHA256"], err = fileSHA(ctx, compiler, 256<<20)
	if err != nil {
		return "", err
	}
	for _, header := range []string{"usr/include/sys/spawn.h", "usr/include/notify.h"} {
		observations[header], err = fileSHA(ctx, filepath.Join(sdk, header), 1<<20)
		if err != nil {
			return "", err
		}
	}
	if _, err = os.Lstat(binary); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("supervisor output must not exist before compilation")
	}
	arch := "arm64"
	if architecture == "amd64" {
		arch = "x86_64"
	}
	if _, err = observe("compile", compiler, "-Wall", "-Wextra", "-Werror", "-O2", "-arch", arch, "-isysroot", sdk, source, "-o", binary); err != nil {
		return "", err
	}
	if err = os.Chmod(binary, 0700); err != nil {
		return "", err
	}
	image, err := macho.Open(binary)
	if err != nil {
		return "", err
	}
	expectedCPU := macho.CpuArm64
	if architecture == "amd64" {
		expectedCPU = macho.CpuAmd64
	}
	validImage := image.Cpu == expectedCPU && image.Type == macho.TypeExec
	if err = image.Close(); err != nil {
		return "", err
	}
	if !validImage {
		return "", errors.New("supervisor is not the selected native executable architecture")
	}
	observations["binarySHA256"], err = fileSHA(ctx, binary, 16<<20)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(observations, "", "  ")
	if err != nil {
		return "", err
	}
	if err = writeNew(filepath.Join(root, "supervisor-build.json"), append(data, '\n')); err != nil {
		return "", err
	}
	current, err := fileSHA(ctx, source, 1<<20)
	if err != nil || current != observations["sourceSHA256"] {
		return "", errors.New("supervisor source changed during compilation")
	}
	current, err = fileSHA(ctx, binary, 16<<20)
	if err != nil || current != observations["binarySHA256"] {
		return "", errors.New("supervisor binary changed before execution")
	}
	current, err = fileSHA(ctx, compiler, 256<<20)
	if err != nil || current != observations["compilerSHA256"] {
		return "", errors.New("selected compiler changed during compilation")
	}
	for _, header := range []string{"usr/include/sys/spawn.h", "usr/include/notify.h"} {
		current, err = fileSHA(ctx, filepath.Join(sdk, header), 1<<20)
		if err != nil || current != observations[header] {
			return "", errors.New("selected supervisor SDK headers changed during compilation")
		}
	}
	return binary, nil
}

func nativeRecord(ctx context.Context, o *Options, root string, workload []string) (inv Invocation, s *Supervision) {
	inv.Exit = -1
	defer func() {
		data, err := json.Marshal(inv)
		if err == nil {
			err = writeNew(filepath.Join(root, "record.status.json"), append(data, '\n'))
		}
		if err != nil {
			inv.Failure = errors.Join(errors.New(inv.Failure), err).Error()
		}
	}()
	binary, err := prepareSupervisor(ctx, o, root)
	if err != nil {
		inv.Failure = err.Error()
		return
	}
	xcrun := o.Xcrun
	if xcrun == "" {
		xcrun = "xcrun"
	}
	xcrun, err = exec.LookPath(xcrun)
	if err == nil {
		xcrun, err = filepath.Abs(xcrun)
	}
	if err != nil {
		inv.Failure = err.Error()
		return
	}
	ms := func(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }
	args := []string{root, o.Directory, ms(o.ReadinessTimeout), ms(o.CommandTimeout), ms(o.CleanupTimeout), strconv.FormatInt(o.MaxArtifactBytes, 10), xcrun, ms(o.TimeLimit) + "ms"}
	args = append(args, workload...)
	inv.Arguments = append([]string{binary}, args...)
	inv, s = executeSupervisor(ctx, o, root, binary, args)
	if s != nil {
		actual := Invocation{Arguments: []string{xcrun, "xctrace", "record", "--template", "Time Profiler", "--time-limit", ms(o.TimeLimit) + "ms", "--no-prompt", "--output", filepath.Join(root, "capture.trace"), "--notify-tracing-started", s.Notification, "--attach", strconv.Itoa(s.TargetPID)}, Exit: inv.Exit, Failure: inv.Failure}
		data, err := json.Marshal(actual)
		if err == nil {
			err = writeNew(filepath.Join(root, "recorder-command.json"), append(data, '\n'))
		}
		if err != nil {
			inv.Failure = errors.Join(errors.New(inv.Failure), err).Error()
		}
	}
	return
}

func executeSupervisor(ctx context.Context, o *Options, root, binary string, args []string) (inv Invocation, s *Supervision) {
	inv = Invocation{Exit: -1, Arguments: append([]string{binary}, args...)}
	if err := ctx.Err(); err != nil {
		inv.Failure = err.Error()
		return
	}
	controlRead, controlWrite, err := os.Pipe()
	if err != nil {
		inv.Failure = err.Error()
		return
	}
	defer func() {
		if err := controlRead.Close(); err != nil {
			inv.Failure = errors.Join(errors.New(inv.Failure), err).Error()
		}
	}()
	stdout, err := os.OpenFile(filepath.Join(root, "supervisor.stdout"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		controlWrite.Close()
		inv.Failure = err.Error()
		return
	}
	stderr, err := os.OpenFile(filepath.Join(root, "supervisor.stderr"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		controlWrite.Close()
		stdout.Close()
		inv.Failure = err.Error()
		return
	}
	// Do not CommandContext-kill the helper before it cleans its children.
	bounded, cancel := context.WithTimeout(ctx, o.CommandTimeout)
	defer cancel()
	cmd := exec.Command(binary, args...)
	cmd.Stdin = controlRead
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdout = &cappedWriter{stdout, 4096, cancel}
	cmd.Stderr = &cappedWriter{stderr, o.MaxArtifactBytes, cancel}
	if err = cmd.Start(); err != nil {
		controlWrite.Close()
		stdout.Close()
		stderr.Close()
		inv.Failure = err.Error()
		return
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-bounded.Done():
		closeErr := controlWrite.Close()
		controlWrite = nil
		select {
		case err = <-done:
		case <-time.After(2*o.CleanupTimeout + 3*time.Second):
			// This is only the directly owned, unreaped helper, not any target.
			// Abnormal helper death gives NO target cleanup guarantee.
			killErr := cmd.Process.Kill()
			select {
			case err = <-done:
			case <-time.After(2 * time.Second):
				err = errors.New("direct helper reap unresolved after kill")
			}
			err = errors.Join(errors.New("supervisor cleanup deadline exceeded; target cleanup unresolved"), killErr, err)
		}
		err = errors.Join(err, bounded.Err(), closeErr)
	}
	if controlWrite != nil {
		err = errors.Join(err, controlWrite.Close())
	}
	err = errors.Join(err, stdout.Close(), stderr.Close())
	data, readErr := readRegular(filepath.Join(root, "supervisor.stdout"), 4096)
	if readErr == nil {
		s, readErr = decodeSupervision(data)
	}
	if s != nil {
		inv.Exit = s.RecorderStatus >> 8
		if s.RecorderStatus&127 != 0 {
			inv.Exit = -1
		}
	}
	if err != nil || readErr != nil || s == nil || !s.valid() {
		inv.Failure = errors.Join(err, readErr, errors.New("supervised record lifecycle not qualified")).Error()
	}
	return
}

func attachedTarget(data []byte, pid int, path string) error {
	root, err := parseXML(data)
	if err != nil {
		return err
	}
	runs := children(root, "run")
	if root.name != "trace-toc" || len(runs) != 1 || runs[0].attrs["number"] != "1" {
		return errors.New("attached target requires exactly fresh run 1")
	}
	info := children(runs[0], "info")
	if len(info) != 1 {
		return errors.New("missing or ambiguous target info")
	}
	targets := children(info[0], "target")
	if len(targets) != 1 {
		return errors.New("missing or ambiguous attached target")
	}
	processes := children(targets[0], "process")
	if len(processes) != 1 || processes[0].attrs["type"] != "attached" || processes[0].attrs["pid"] != strconv.Itoa(pid) || processes[0].attrs["return-exit-status"] != "0" {
		return errors.New("trace does not identify successful exact attached target")
	}
	sets := children(runs[0], "processes")
	if len(sets) != 1 {
		return errors.New("missing or ambiguous process inventory")
	}
	count := 0
	for _, process := range children(sets[0], "process") {
		if process.attrs["pid"] == strconv.Itoa(pid) {
			if process.attrs["path"] != path {
				return errors.New("attached target executable path differs")
			}
			count++
		}
	}
	if count != 1 {
		return errors.New("missing or ambiguous exact attached executable identity")
	}
	return nil
}
