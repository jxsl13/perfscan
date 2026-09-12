package allocationcampaign

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CaptureContext retains native streams and a nonzero exit on cancellation.
// WaitDelay bounds inherited-pipe waits; cancellation is failed evidence.
func CaptureContext(ctx context.Context, timeout time.Duration, dir, name, root string, env []string, binary string, args ...string) (RawInvocation, error) {
	if ctx == nil || timeout <= 0 || name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\") {
		return RawInvocation{}, errors.New("invalid bounded measurement capture")
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(bounded, binary, args...)
	command.Dir = root
	command.Env = env
	command.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	code := 0
	if err != nil {
		code = -1
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			code = exited.ExitCode()
		}
	}
	if bounded.Err() != nil {
		code = -1
		stderr.WriteString("\nperfscan measurement context: " + bounded.Err().Error() + "\n")
	}
	out, errout := stdout.Bytes(), stderr.Bytes()
	if err := artifact(dir, name, out, errout, code); err != nil {
		return RawInvocation{}, err
	}
	return RawInvocation{Exit: code, StdoutSHA256: hash(out), StderrSHA256: hash(errout)}, nil
}
