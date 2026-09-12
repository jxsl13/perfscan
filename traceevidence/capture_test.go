package traceevidence

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These subprocesses simulate command outcomes only. They never profile a
// workload, invoke Instruments, or modify privacy permissions.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "xctrace" {
		os.Exit(fakeXctrace(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func fakeArg(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func fakeWrite(path, value string) {
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		panic(err)
	}
}

func fakeXctrace(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "version":
		if data, err := os.ReadFile("synthetic-version-mode"); err == nil {
			if string(data) == "empty" {
				return 0
			}
			if string(data) == "failed" {
				fmt.Fprintln(os.Stderr, "synthetic version failure")
				return 9
			}
		}
		fmt.Println("Synthetic xctrace 17F113")
		return 0
	case "record":
		mode := args[len(args)-1]
		trace, target := fakeArg(args, "--output"), fakeArg(args, "--target-stdout")
		if mode == "preexisting-result" {
			fakeWrite(filepath.Join(filepath.Dir(trace), "result.json"), "preserve existing result")
		}
		fmt.Fprintln(os.Stderr, "synthetic recorder diagnostics")
		if mode == "cancel" {
			fmt.Println("record began")
			time.Sleep(time.Minute)
			return 0
		}
		if mode == "output-limit" {
			fmt.Print(strings.Repeat("x", 4096))
			return 0
		}
		if mode != "missing-trace" {
			if err := os.Mkdir(trace, 0700); err != nil {
				panic(err)
			}
			if mode != "empty-trace" {
				fakeWrite(filepath.Join(trace, "mode"), mode)
			}
			if mode == "symlink-trace" {
				if err := os.Symlink(target, filepath.Join(trace, "link")); err != nil {
					panic(err)
				}
			}
			if mode == "large-trace" {
				fakeWrite(filepath.Join(trace, "large"), strings.Repeat("x", 8192))
			}
		}
		markers := "START\nDONE\n"
		switch mode {
		case "missing-start":
			markers = "DONE\n"
		case "missing-completion":
			markers = "START\n"
		case "reverse-markers":
			markers = "DONE\nSTART\n"
		case "duplicate-start":
			markers = "START\nSTART\nDONE\n"
		case "substring-markers":
			markers = "prefix START\nprefix DONE\n"
		case "large-target":
			markers = strings.Repeat("x", 8192) + markers
		}
		if mode != "missing-target" {
			fakeWrite(target, markers)
		}
		if mode == "symlink-target" {
			if err := os.Remove(target); err != nil {
				panic(err)
			}
			if err := os.Symlink(filepath.Join(trace, "mode"), target); err != nil {
				panic(err)
			}
		}
		if strings.HasPrefix(mode, "54") {
			if strings.HasPrefix(mode, "54-native") {
				fmt.Println("Reached specified time limit, ending recording...")
				fmt.Println("Recording completed. Saving output file...")
				fmt.Println("Output file saved as: capture.trace")
				switch mode {
				case "54-native-conflicting-path":
					fmt.Println("Output file saved as: unrelated.trace")
				case "54-native-duplicate-saved":
					fmt.Println("Output file saved as: capture.trace")
				case "54-native-duplicate-limit":
					fmt.Println("Reached specified time limit, ending recording...")
				case "54-native-duplicate-completed":
					fmt.Println("Recording completed. Saving output file...")
				}
				return 54
			}
			if mode != "54-missing-limit" {
				fmt.Println("Reached specified time limit")
			}
			if mode != "54-missing-completed" {
				fmt.Println("Recording completed")
			}
			if mode != "54-missing-saved" {
				saved := trace
				if mode == "54-wrong-path" {
					saved += ".other"
				}
				fmt.Println("Output file saved as: " + saved)
			}
			if mode == "54-duplicate-limit" {
				fmt.Println("Reached specified time limit")
			}
			return 54
		}
		if mode == "nonzero" {
			return 7
		}
		return 0
	case "export":
		trace := fakeArg(args, "--input")
		data, err := os.ReadFile(filepath.Join(trace, "mode"))
		if err != nil {
			return 3
		}
		mode := string(data)
		if mode == "export-failed" {
			fmt.Fprintln(os.Stderr, "synthetic export failed")
			return 8
		}
		if fakeArg(args, "--xpath") == "" {
			toc := `<trace-toc><run number="1"><data><table schema="gpu-counter"/></data></run></trace-toc>`
			if strings.HasPrefix(mode, "native-") {
				toc = strings.Replace(toc, `<table schema="gpu-counter"/>`, `<table schema="unrelated-events"/><table schema="gpu-counter"/>`, 1)
			}
			switch mode {
			case "malformed-toc":
				toc = `<trace-toc>`
			case "trailing-toc":
				toc += "garbage"
			case "wrong-run":
				toc = strings.ReplaceAll(toc, `number="1"`, `number="2"`)
			case "missing-schema":
				toc = strings.ReplaceAll(toc, "gpu-counter", "other")
			case "duplicate-schema":
				toc = strings.ReplaceAll(toc, `<table schema="gpu-counter"/>`, `<table schema="gpu-counter"/><table schema="gpu-counter"/>`)
			case "namespace-toc":
				toc = strings.Replace(toc, "<trace-toc>", `<trace-toc xmlns="other">`, 1)
			}
			fmt.Print(toc)
			return 0
		}
		if mode == "mutate-trace" {
			fakeWrite(filepath.Join(trace, "mode"), "changed")
		}
		if mode == "mutate-target" {
			fakeWrite(filepath.Join(filepath.Dir(trace), "target.txt"), "START\nDONE\nchanged")
		}
		var escaped strings.Builder
		if err := xml.EscapeText(&escaped, []byte(fakeArg(args, "--xpath"))); err != nil {
			panic(err)
		}
		xpath := escaped.String()
		cols := `<col><mnemonic>time</mnemonic></col><col><mnemonic>value</mnemonic></col>`
		rows := `<row><time id="1">100</time><value id="2">42</value></row><row><time ref="1"/><value ref="2"/></row>`
		if strings.HasPrefix(mode, "native-") {
			xpath = `//trace-toc[1]/run[1]/data[1]/table[2]`
			if mode == "native-single-slash" {
				xpath = strings.TrimPrefix(xpath, "/")
			}
			if mode == "native-wrong-table" {
				xpath = strings.Replace(xpath, "table[2]", "table[1]", 1)
			}
			if mode == "native-wrong-run" {
				xpath = strings.Replace(xpath, "run[1]", "run[2]", 1)
			}
			cols = `<col><mnemonic>time</mnemonic><engineering-type>timestamp</engineering-type></col><col><mnemonic>value</mnemonic><engineering-type>scalar</engineering-type></col><col><mnemonic>device</mnemonic><engineering-type>device</engineering-type></col>`
			rows = `<row><timestamp id="11">1200</timestamp><scalar id="12">7.5</scalar><device id="13"><name>Synthetic GPU</name></device></row><row><timestamp ref="11"/><scalar ref="12"/><device ref="13"/></row>`
		}
		switch mode {
		case "missing-column":
			cols = `<col><mnemonic>time</mnemonic></col>`
			rows = `<row><time>100</time></row>`
		case "missing-rows":
			rows = ""
		case "truncated-row":
			rows = `<row><time>100</time></row>`
		case "unresolved-ref":
			rows = `<row><time ref="99"/><value>42</value></row>`
		case "empty-cell":
			rows = `<row><time/><value/></row>`
		case "wrongtype-ref":
			rows = `<row><time id="1">100</time><value ref="1"/></row>`
		case "wrong-query":
			xpath = strings.ReplaceAll(xpath, "gpu-counter", "other")
		}
		table := `<trace-query-result><node xpath="` + xpath + `"><schema name="gpu-counter">` + cols + `</schema>` + rows + `</node></trace-query-result>`
		if mode == "wrong-table-schema" {
			table = strings.ReplaceAll(table, `name="gpu-counter"`, `name="other"`)
		}
		if mode == "malformed-table" {
			table = strings.TrimSuffix(table, "</trace-query-result>")
		}
		if mode == "trailing-table" {
			table += `<extra/>`
		}
		fmt.Print(table)
		return 0
	}
	return 2
}

func syntheticOptions(t *testing.T, mode string) *Options {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &Options{Xcrun: executable, Output: filepath.Join(t.TempDir(), "evidence"), Workload: []string{"synthetic-workload", mode}, Instruments: []string{"Metal"}, Schemas: []Schema{{Name: "gpu-counter", Columns: []string{"time", "value"}}}, Started: "START", Completed: "DONE", TimeLimit: time.Millisecond, CommandTimeout: 5 * time.Second, MaxArtifactBytes: 4096, MaxTraceBytes: 4096}
}

func TestCaptureSyntheticAcceptance(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"0", "54", "54-native", "native-positional", "native-single-slash"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, mode)
			result, err := Capture(context.Background(), o)
			if err != nil || result == nil || !result.Accepted || result.TimeLimited != strings.HasPrefix(mode, "54") || len(result.TraceSHA256) != 64 || result.TableRows["gpu-counter"] != 2 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			for _, name := range []string{"version", "record", "toc", "table-0"} {
				inv, ok := result.Invocations[name]
				if !ok || inv.Failure != "" {
					t.Fatalf("missing successful %s: %+v", name, inv)
				}
				for _, suffix := range []string{".stdout", ".stderr", ".status.json"} {
					if _, err := os.Stat(filepath.Join(o.Output, name+suffix)); err != nil {
						t.Fatal(err)
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(o.Output, "record.status.json"))
			if err != nil {
				t.Fatal(err)
			}
			var status Invocation
			if err = json.Unmarshal(data, &status); err != nil || status.Exit != result.Invocations["record"].Exit {
				t.Fatalf("status=%+v err=%v", status, err)
			}
		})
	}
}

func TestCaptureSyntheticRejections(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"nonzero", "54-missing-limit", "54-missing-completed", "54-missing-saved", "54-wrong-path", "54-duplicate-limit", "missing-start", "missing-completion", "reverse-markers", "duplicate-start", "substring-markers", "missing-trace", "empty-trace", "symlink-trace", "large-trace", "missing-target", "symlink-target", "large-target", "malformed-toc", "trailing-toc", "wrong-run", "missing-schema", "duplicate-schema", "namespace-toc", "missing-column", "missing-rows", "truncated-row", "unresolved-ref", "empty-cell", "wrongtype-ref", "native-wrong-table", "native-wrong-run", "wrong-query", "wrong-table-schema", "malformed-table", "trailing-table", "export-failed", "mutate-trace", "mutate-target"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, mode)
			result, err := Capture(context.Background(), o)
			if err == nil || result == nil || result.Accepted || result.Reason == "" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			for _, file := range []string{"result.json", "record.stderr", "record.status.json"} {
				if _, err := os.Stat(filepath.Join(o.Output, file)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCapturePreservesExistingDirectory(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	if err := os.Mkdir(o.Output, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(o.Output, "existing")
	fakeWrite(sentinel, "keep")
	if result, err := Capture(context.Background(), o); err == nil || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatalf("existing content=%q err=%v", data, err)
	}
	entries, err := os.ReadDir(o.Output)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory changed: %v %v", entries, err)
	}
}

func TestCaptureRequiresObservedVersion(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"empty", "failed"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, "0")
			o.Directory = t.TempDir()
			fakeWrite(filepath.Join(o.Directory, "synthetic-version-mode"), mode)
			result, err := Capture(context.Background(), o)
			if err == nil || result == nil || result.Accepted {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if _, ok := result.Invocations["record"]; ok {
				t.Fatal("record started without observed version")
			}
			for _, name := range []string{"plan.json", "result.json", "version.status.json", "version.stdout", "version.stderr"} {
				if _, err := os.Stat(filepath.Join(o.Output, name)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCaptureRejectsConflictingNativeTimeoutMarkers(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"54-native-conflicting-path", "54-native-duplicate-saved", "54-native-duplicate-limit", "54-native-duplicate-completed"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, mode)
			result, err := Capture(context.Background(), o)
			if err == nil || result == nil || result.Accepted {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if result.Invocations["record"].Exit != 54 || !strings.Contains(result.Reason, "markers") {
				t.Fatalf("not rejected by qualified marker check: %+v", result)
			}
			if _, ok := result.Invocations["toc"]; ok {
				t.Fatal("exported conflicting timeout capture")
			}
		})
	}
}

func TestCaptureRejectsResultPersistenceConflict(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "preexisting-result")
	result, err := Capture(context.Background(), o)
	if err == nil || result == nil || result.Accepted {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.TableRows["gpu-counter"] != 2 {
		t.Fatal("fixture failed before persistence boundary")
	}
	data, readErr := os.ReadFile(filepath.Join(o.Output, "result.json"))
	if readErr != nil || string(data) != "preserve existing result" {
		t.Fatalf("existing result changed: %q %v", data, readErr)
	}
}

func TestRunCommandRetainsCancellationAndLimits(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"cancel", "output-limit"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, mode)
			if err := os.Mkdir(o.Output, 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			prefix := filepath.Join(o.Output, "record")
			inv := runCommand(ctx, o.Xcrun, "", prefix, []string{"xctrace", "record", mode}, 128)
			if inv.Failure == "" {
				t.Fatalf("failure not retained: %+v", inv)
			}
			data, err := os.ReadFile(prefix + ".stdout")
			if err != nil || len(data) > 128 {
				t.Fatalf("stdout=%q err=%v", data, err)
			}
			if mode == "output-limit" && len(data) != 128 {
				t.Fatalf("bounded prefix length=%d", len(data))
			}
			if _, err := os.Stat(prefix + ".status.json"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
