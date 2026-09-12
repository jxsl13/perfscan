package allocationcampaign

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictSamples(t *testing.T) {
	t.Parallel()
	valid := `{"n":1024,"memBytes":1025,"memAllocs":1023,"bytesPerOp":1,"bytesRemainder":1,"allocsPerOp":0,"allocsRemainder":1023}`
	for _, tc := range []struct {
		name, text string
		valid      bool
	}{
		{"valid", valid + "\nPASS\n", true},
		{"duplicate", strings.Replace(valid, `"n":1024`, `"n":1,"n":1024`, 1) + "\nPASS\n", false},
		{"null", strings.Replace(valid, `"allocsPerOp":0`, `"allocsPerOp":null`, 1) + "\nPASS\n", false},
		{"float", strings.Replace(valid, `"n":1024`, `"n":1024.0`, 1) + "\nPASS\n", false},
		{"overflow", strings.Replace(valid, `"memBytes":1025`, `"memBytes":1e999`, 1) + "\nPASS\n", false},
		{"missing", strings.Replace(valid, `"allocsPerOp":0,`, "", 1) + "\nPASS\n", false},
		{"unknown", strings.Replace(valid, `"n":1024`, `"n":1024,"unknown":1`, 1) + "\nPASS\n", false},
		{"identity", strings.Replace(valid, `"bytesRemainder":1`, `"bytesRemainder":2`, 1) + "\nPASS\n", false},
		{"two", valid + "\n" + valid + "\nPASS\n", false},
		{"failed", valid + "\nFAIL\n", false},
		{"nopass", valid + "\n", false},
		{"twopass", valid + "\nPASS\nPASS\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSample([]byte(tc.text))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
	for _, text := range []string{`{"a":{"b":1,"b":2}}`, `{"a":null}`, `{"a":1} {"b":2}`, `{"a":NaN}`} {
		if err := uniqueJSON([]byte(text)); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}

func TestBalancedOrderAndRationalMedians(t *testing.T) {
	t.Parallel()
	inv := invocations(2, []int{1, 12})
	if len(inv) != 16 || inv[0].Arm != "A" || inv[4].Arm != "B" || inv[4].Procs != 12 || inv[6].Procs != 1 {
		t.Fatalf("%+v", inv)
	}
	for _, sample := range inv {
		if sample.Phase == "control" && (sample.Binary != "old.test" || sample.Selection != "before") {
			t.Fatal("control uses candidate")
		}
	}
	if median([]int64{math.MaxInt64, math.MaxInt64}).RatString() != "9223372036854775807" || median([]int64{0, 1}).RatString() != "1/2" {
		t.Fatal("lost exact rational median")
	}
}

func TestSnapshotAndInstrumentationSafeguards(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"../escape", "/absolute", "link"} {
		var archive bytes.Buffer
		writer := tar.NewWriter(&archive)
		header := &tar.Header{Name: name, Mode: 0600}
		if name == "link" {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = "/outside"
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := unpack(archive.Bytes(), t.TempDir()); err == nil {
			t.Fatalf("accepted %s", name)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\ngo 1.25\nreplace dependency => ../mutable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pinnedModule(dir); err == nil {
		t.Fatal("accepted mutable replacement dependency")
	}
	if err := os.Mkdir(filepath.Join(dir, "benchmarkevidence"), 0700); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(dir, "diagnostic_test.go")
	helper := filepath.Join(dir, "benchmarkevidence", "allocation.go")
	for _, path := range []string{wrapper, helper} {
		if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	old, err := instrumentation(dir, []string{"diagnostic_test.go", "benchmarkevidence"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("changed helper only"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate, err := instrumentation(dir, []string{"diagnostic_test.go", "benchmarkevidence"})
	if err != nil {
		t.Fatal(err)
	}
	if old["diagnostic_test.go"] != candidate["diagnostic_test.go"] || old["benchmarkevidence/allocation.go"] == candidate["benchmarkevidence/allocation.go"] {
		t.Fatal("failed to distinguish helper-only instrumentation change")
	}
}

func TestGitSnapshotExcludesIgnoredBuildInput(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Git exports repository-local variables into hooks. A fixture repository
	// must not inherit those index/worktree settings from the real commit.
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	for path, data := range map[string]string{"go.mod": "module fixture\ngo 1.25\n", "tracked.go": "package fixture\n", ".gitignore": "extra.go\n", "extra.go": "package fixture\n// ignored mutable build input\n"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init"}, {"config", "core.autocrlf", "false"}, {"add", "go.mod", "tracked.go", ".gitignore"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", "pin fixture"}, {"config", "core.autocrlf", "true"}} {
		out, errout, code := command(root, env, "git", args...)
		if code != 0 {
			t.Fatalf("git %v: %s %s", args, out, errout)
		}
	}
	status, stderr, code := command(root, env, "git", "status", "--porcelain")
	if code != 0 || len(status) != 0 {
		t.Fatalf("fixture should appear clean despite ignored Go input: exit=%d stdout=%q stderr=%q", code, status, stderr)
	}
	tree, stderr, code := command(root, env, "git", "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if code != 0 {
		t.Fatalf("tree: %s", stderr)
	}
	// Reproduce the Windows default locally: an ordinary archive contains
	// converted CRLF bytes and must fail the unchanged committed-blob check.
	converted, errout, code := command(root, env, "git", "archive", "--format=tar", "HEAD")
	if code != 0 {
		t.Fatalf("converted archive: %s", errout)
	}
	convertedSnapshot := t.TempDir()
	if err := unpack(converted, convertedSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := exactTree(convertedSnapshot, tree); err == nil {
		t.Fatal("accepted checkout-converted source instead of committed blobs")
	}
	archive, errout, code := sourceArchive(root, env, "HEAD")
	if code != 0 {
		t.Fatalf("archive: %s", errout)
	}
	snapshot := t.TempDir()
	if err := unpack(archive, snapshot); err != nil {
		t.Fatal(err)
	}
	configured, stderr, code := command(root, env, "git", "config", "core.autocrlf")
	if code != 0 || strings.TrimSpace(string(configured)) != "true" {
		t.Fatalf("archive changed caller Git configuration: %q %s", configured, stderr)
	}
	if err := exactTree(snapshot, tree); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "extra.go")); !os.IsNotExist(err) {
		t.Fatalf("ignored source entered snapshot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(snapshot, "tracked.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "tracked.go"), []byte("substituted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := exactTree(snapshot, tree); err == nil {
		t.Fatal("accepted substituted committed content")
	}
	if err := os.Remove(filepath.Join(snapshot, "tracked.go")); err != nil {
		t.Fatal(err)
	}
	if err := exactTree(snapshot, tree); err == nil {
		t.Fatal("accepted omitted committed build input")
	}
	// Explicit attributes override ordinary checkout configuration. They must
	// still fail closed when they transform the archived committed bytes.
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("*.go text eol=crlf\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", ".gitattributes"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", "pin explicit eol attribute"}} {
		out, stderr, code := command(root, env, "git", args...)
		if code != 0 {
			t.Fatalf("git %v: %s %s", args, out, stderr)
		}
	}
	attributed, stderr, code := sourceArchive(root, env, "HEAD")
	if code != 0 {
		t.Fatalf("attributed archive: %s", stderr)
	}
	attributedTree, stderr, code := command(root, env, "git", "ls-tree", "-r", "-z", "--full-tree", "HEAD")
	if code != 0 {
		t.Fatalf("attributed tree: %s", stderr)
	}
	attributedSnapshot := t.TempDir()
	if err := unpack(attributed, attributedSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := exactTree(attributedSnapshot, attributedTree); err == nil {
		t.Fatal("accepted attribute-converted committed source")
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	instrument := []byte("// identical diagnostic source\n")
	plan := Plan{Schema: 1, N: N, Pairs: 2, Procs: []int{1, 12}, Package: "./benchmarks", Diagnostic: "benchmarks/allocation_diagnostic_test.go", Invocations: invocations(2, []int{1, 12})}
	plan.InstrumentationRoots = []string{plan.Diagnostic, "benchmarkevidence"}
	plan.RuntimeEnvironment = map[string]string{"GODEBUG": "", "GOGC": "", "GOMEMLIMIT": ""}
	var archive bytes.Buffer
	var tree bytes.Buffer
	writer := tar.NewWriter(&archive)
	for path, data := range map[string][]byte{plan.Diagnostic: instrument, "benchmarkevidence/allocation.go": instrument, "go.mod": []byte("module fixture\ngo 1.25\n")} {
		if err := writer.WriteHeader(&tar.Header{Name: path, Size: int64(len(data)), Mode: 0600}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
		blob := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
		_, _ = fmt.Fprintf(&tree, "100644 blob %s\t%s\x00", hash(blob), path)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old", "candidate"} {
		binary := []byte(name)
		commit := strings.Repeat("a", 40)
		plan.Builds = append(plan.Builds, Build{Root: "/pinned/" + name, Commit: commit, Instrumentation: map[string]string{plan.Diagnostic: hash(instrument), "benchmarkevidence/allocation.go": hash(instrument)}, ArchiveSHA256: hash(archive.Bytes()), TreeSHA256: hash(tree.Bytes()), Environment: "{}\n", Binary: name + ".test", SHA256: hash(binary)})
		for path, data := range map[string][]byte{name + ".test": binary} {
			if err := os.WriteFile(filepath.Join(dir, path), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		for _, kind := range []string{"status", "commit", "environment", "build", "postbuild-status", "postbuild-commit", "archive", "tree"} {
			var out []byte
			if kind == "commit" || kind == "postbuild-commit" {
				out = []byte(commit + "\n")
			}
			if kind == "environment" {
				out = []byte("{}\n")
			}
			if kind == "archive" {
				out = archive.Bytes()
			}
			if kind == "tree" {
				out = tree.Bytes()
			}
			if err := artifact(dir, name+"-"+kind, out, nil, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writeJSON(filepath.Join(dir, "plan.json"), plan); err != nil {
		t.Fatal(err)
	}
	var records []Record
	for i, inv := range plan.Invocations {
		out := []byte(`{"n":1024,"memBytes":1025,"memAllocs":1023,"bytesPerOp":1,"bytesRemainder":1,"allocsPerOp":0,"allocsRemainder":1023}` + "\nPASS\n")
		if inv.Arm == "B" {
			out = bytes.Replace(out, []byte(`"memBytes":1025`), []byte(`"memBytes":1026`), 1)
			out = bytes.Replace(out, []byte(`"bytesRemainder":1`), []byte(`"bytesRemainder":2`), 1)
		}
		if err := artifact(dir, invocationName(i), out, nil, 0); err != nil {
			t.Fatal(err)
		}
		records = append(records, Record{inv, 0, hash(out), hash(nil)})
	}
	if err := writeJSON(filepath.Join(dir, "records.json"), records); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestIndependentVerification(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"valid", "missing", "tampered", "exit", "binary", "instrumentation", "order", "duplicate", "incomplete", "buildstderr", "extra", "helperonly"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			dir := fixture(t)
			planBytes, err := os.ReadFile(filepath.Join(dir, "plan.json"))
			if err != nil {
				t.Fatal(err)
			}
			recordBytes, err := os.ReadFile(filepath.Join(dir, "records.json"))
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "helperonly":
				var plan Plan
				if err := readJSON(filepath.Join(dir, "plan.json"), &plan); err != nil {
					t.Fatal(err)
				}
				plan.Builds[1].Instrumentation["benchmarkevidence/allocation.go"] = hash([]byte("changed"))
				if err := writeJSON(filepath.Join(dir, "plan.json"), plan); err != nil {
					t.Fatal(err)
				}
				planBytes, err = os.ReadFile(filepath.Join(dir, "plan.json"))
				if err != nil {
					t.Fatal(err)
				}
			case "extra":
				if err := os.WriteFile(filepath.Join(dir, "sample-9999.stdout"), []byte("extra"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err := os.Remove(filepath.Join(dir, "sample-0000.stdout")); err != nil {
					t.Fatal(err)
				}
			case "buildstderr":
				if err := os.Remove(filepath.Join(dir, "old-build.stderr")); err != nil {
					t.Fatal(err)
				}
			case "tampered", "exit", "binary", "instrumentation":
				path := map[string]string{"tampered": "sample-0000.stdout", "exit": "sample-0000.exit", "binary": "old.test", "instrumentation": "old-archive.stdout"}[mode]
				if err := os.WriteFile(filepath.Join(dir, path), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "order", "duplicate", "incomplete":
				var records []Record
				if err := readJSON(filepath.Join(dir, "records.json"), &records); err != nil {
					t.Fatal(err)
				}
				if mode == "incomplete" {
					records = records[:len(records)-1]
				} else if mode == "duplicate" {
					records[1] = records[0]
				} else {
					records[0], records[1] = records[1], records[0]
				}
				if err := writeJSON(filepath.Join(dir, "records.json"), records); err != nil {
					t.Fatal(err)
				}
			}
			result, err := Verify(dir, hash(planBytes), hash(recordBytes))
			if mode != "valid" {
				if err == nil {
					t.Fatal("accepted invalid evidence")
				}
				return
			}
			if err != nil || len(result.Pairs) != 8 || len(result.RoundedArmMedians) != 4 {
				t.Fatalf("%+v %v", result, err)
			}
			for _, pair := range result.Pairs {
				if pair.Delta.MemBytes != 1 {
					t.Fatalf("%+v", pair)
				}
			}
			for _, cell := range result.RoundedArmMedians {
				if cell.BytesDifference != "0" {
					t.Fatalf("%+v", cell)
				}
			}
			if _, err := json.Marshal(result); err != nil {
				t.Fatal(err)
			}
		})
	}
}
