package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscanxx/internal/catalog"
)

// runCLI drives the real entry point and returns (stdout, stderr, exit).
func runCLI(args ...string) (string, string, int) {
	var out, errBuf bytes.Buffer
	code := run(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func catalogCounts() (total, fixable int) {
	for _, e := range catalog.All() {
		total++
		if e.HasFix {
			fixable++
		}
	}
	return
}

func TestListShowsEveryCheckAndSummary(t *testing.T) {
	out, _, code := runCLI("-list")
	if code != 0 {
		t.Fatalf("-list exit = %d, want 0", code)
	}
	total, fixable := catalogCounts()
	for _, e := range catalog.All() {
		if !strings.Contains(out, e.ID) {
			t.Errorf("-list output missing check %s", e.ID)
		}
	}
	// Footer must report the true fix-coverage split.
	for _, want := range []string{
		"checks", "auto-fixable", "advisory",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("-list footer missing %q", want)
		}
	}
	if !strings.Contains(out, itoa(total)) || !strings.Contains(out, itoa(fixable)) {
		t.Errorf("-list footer must mention total=%d and fixable=%d; got:\n%s", total, fixable, out)
	}
}

func TestListFixableFiltersToAutoFixOnly(t *testing.T) {
	out, _, code := runCLI("-list", "-fixable")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	for _, e := range catalog.All() {
		listed := strings.Contains(out, e.ID)
		if e.HasFix && !listed {
			t.Errorf("-fixable dropped auto-fix check %s", e.ID)
		}
		if !e.HasFix && listed {
			t.Errorf("-fixable leaked advisory check %s", e.ID)
		}
	}
}

func TestListJSONIsValidAndComplete(t *testing.T) {
	out, _, code := runCLI("-list", "-json")
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	var got []struct {
		ID       string `json:"id"`
		Level    int    `json:"level"`
		TidyName string `json:"tidyCheck"`
		AutoFix  bool   `json:"autoFix"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("-list -json is not valid JSON: %v", err)
	}
	total, fixable := catalogCounts()
	if len(got) != total {
		t.Errorf("-json has %d entries, want %d", len(got), total)
	}
	af := 0
	for _, c := range got {
		if c.AutoFix {
			af++
		}
		if c.ID == "" || c.TidyName == "" || c.Level < 1 {
			t.Errorf("-json entry incomplete: %+v", c)
		}
	}
	if af != fixable {
		t.Errorf("-json autoFix count = %d, want %d", af, fixable)
	}

	// -fixable narrows the JSON too.
	fx, _, _ := runCLI("-list", "-json", "-fixable")
	var fxGot []struct{}
	if err := json.Unmarshal([]byte(fx), &fxGot); err != nil {
		t.Fatalf("-list -json -fixable not valid JSON: %v", err)
	}
	if len(fxGot) != fixable {
		t.Errorf("-json -fixable has %d entries, want %d", len(fxGot), fixable)
	}
}

func TestExplainBuildsCorrectUpstreamURLPerFamily(t *testing.T) {
	cases := []struct{ id, want string }{
		{"PX1001", "/checks/performance/for-range-copy.html"},
		{"PX3009", "/checks/readability/redundant-string-cstr.html"},
		{"PX2003", "/checks/modernize/use-emplace.html"},
		{"PX3015", "/checks/cppcoreguidelines/prefer-member-initializer.html"},
	}
	for _, c := range cases {
		out, _, code := runCLI("-explain", c.id)
		if code != 0 {
			t.Errorf("-explain %s exit = %d", c.id, code)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("-explain %s: want URL containing %q, got:\n%s", c.id, c.want, out)
		}
	}
	// Query-based custom check has no upstream page.
	out, _, _ := runCLI("-explain", "PX2101")
	if strings.Contains(out, "clang.llvm.org") {
		t.Errorf("-explain PX2101 should NOT print an upstream URL, got:\n%s", out)
	}
	if !strings.Contains(out, "perfscanxx-defined") {
		t.Errorf("-explain PX2101 should note it is perfscanxx-defined, got:\n%s", out)
	}
}

func TestExplainUnknownCheckFails(t *testing.T) {
	_, errOut, code := runCLI("-explain", "PX9999")
	if code == 0 {
		t.Error("-explain PX9999: want non-zero exit")
	}
	if !strings.Contains(errOut, "unknown check") {
		t.Errorf("-explain PX9999: want 'unknown check' on stderr, got %q", errOut)
	}
}

// itoa avoids importing strconv just for the footer assertion.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
