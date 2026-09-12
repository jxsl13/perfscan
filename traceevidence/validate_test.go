package traceevidence

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseXMLRejectsIncompleteOrAmbiguousEvidence(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"", "<a>", "<a/></a>", "<a/><b/>", "<a/>junk", "junk<a/>", `<a xmlns="urn:other"/>`, `<a x="1" x="2"/>`, `<!DOCTYPE a><a/>`, `<?other anything?><a/>`, `<?xml version="1.0"?><?xml version="1.0"?><a/>`, strings.Repeat("<a>", 129) + strings.Repeat("</a>", 129)} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if _, err := parseXML([]byte(source)); err == nil {
				t.Fatal("accepted unsupported XML")
			}
		})
	}
}

func TestValidateTableIdentityAndValues(t *testing.T) {
	t.Parallel()
	xpath := `/trace-toc/run[@number="1"]/data/table[@schema="gpu-counter"]`
	var escaped strings.Builder
	if err := xml.EscapeText(&escaped, []byte(xpath)); err != nil {
		t.Fatal(err)
	}
	valid := `<trace-query-result><node xpath="` + escaped.String() + `"><schema name="gpu-counter"><col><mnemonic>time</mnemonic></col><col><mnemonic>value</mnemonic></col></schema><row><time id="1">1</time><value id="2">2</value></row><row><time ref="1"/><value ref="2"/></row></node></trace-query-result>`
	required := Schema{Name: "gpu-counter", Columns: []string{"time", "value"}}
	if n, err := validateTable([]byte(valid), xpath, required); err != nil || n != 2 {
		t.Fatalf("rows=%d err=%v", n, err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"wrong root", "trace-query-result", "other"},
		{"wrong selected run", "number=&#34;1&#34;", "number=&#34;2&#34;"},
		{"wrong schema", `name="gpu-counter"`, `name="other"`},
		{"duplicate column", "<mnemonic>value</mnemonic>", "<mnemonic>time</mnemonic>"},
		{"missing mnemonic", "<mnemonic>value</mnemonic>", ""},
		{"duplicate identity", `id="2"`, `id="1"`},
		{"invalid identity", `id="2"`, `id="not-number"`},
		{"invalid reference", `ref="2"`, `ref="not-number"`},
		{"unresolved reference", `ref="2"`, `ref="99"`},
		{"reference with text", `<value ref="2"/>`, `<value ref="2">text</value>`},
		{"reference with child", `<value ref="2"/>`, `<value ref="2"><value>text</value></value>`},
		{"reference with identity", `ref="2"`, `ref="2" id="3"`},
		{"wrong reference element type", `<value ref="2"/>`, `<time ref="2"/>`},
		{"empty required value", `<value id="2">2</value>`, `<value id="2"/>`},
		{"cyclic required payload", `<value id="2">2</value>`, `<value id="2"><value ref="2"/></value>`},
		{"truncated required row", `<time ref="1"/><value ref="2"/>`, `<time ref="1"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			changed := strings.ReplaceAll(valid, tc.from, tc.to)
			if changed == valid {
				t.Fatal("mutation did not change fixture")
			}
			if _, err := validateTable([]byte(changed), xpath, required); err == nil {
				t.Fatal("accepted invalid table")
			}
		})
	}
}

func TestCaptureCanceledBeforeStartLeavesNoDirectory(t *testing.T) {
	t.Parallel()
	o := syntheticOptions(t, "0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := syntheticCapture(ctx, o); err == nil || result != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
		t.Fatalf("created output for canceled capture: %v", err)
	}
}

func TestCaptureInvalidOptionsDoNotCreateArtifacts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*Options)
	}{
		{"no workload", func(o *Options) { o.Workload = nil }},
		{"no instruments", func(o *Options) { o.Instruments = nil }},
		{"no schemas", func(o *Options) { o.Schemas = nil }},
		{"duplicate schema", func(o *Options) { o.Schemas = append(o.Schemas, o.Schemas[0]) }},
		{"unsafe schema selector", func(o *Options) { o.Schemas[0].Name = `gpu-counter"]` }},
		{"duplicate column", func(o *Options) { o.Schemas[0].Columns = []string{"time", "time"} }},
		{"same markers", func(o *Options) { o.Completed = o.Started }},
		{"multiline marker", func(o *Options) { o.Started = "START\nother" }},
		{"nonpositive artifact limit", func(o *Options) { o.MaxArtifactBytes = 0 }},
		{"nonpositive trace limit", func(o *Options) { o.MaxTraceBytes = 0 }},
		{"timeout too short", func(o *Options) { o.CommandTimeout = o.TimeLimit }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := syntheticOptions(t, "0")
			tc.change(o)
			if result, err := syntheticCapture(context.Background(), o); err == nil || result != nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
				t.Fatalf("created artifacts for invalid options: %v", err)
			}
		})
	}
}

func TestBundleDigestBindsNamesAndContents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "one")
	fakeWrite(path, "data")
	before, err := bundleDigest(root, 1024)
	if err != nil {
		t.Fatal(err)
	}
	again, err := bundleDigest(root, 1024)
	if err != nil || before != again {
		t.Fatalf("unstable digest: %v", err)
	}
	if err := os.Rename(path, filepath.Join(root, "two")); err != nil {
		t.Fatal(err)
	}
	renamed, err := bundleDigest(root, 1024)
	if err != nil || renamed == before {
		t.Fatalf("rename unbound: %v", err)
	}
	fakeWrite(filepath.Join(root, "two"), "changed")
	changed, err := bundleDigest(root, 1024)
	if err != nil || changed == renamed {
		t.Fatalf("contents unbound: %v", err)
	}
}

func TestBundleDigestHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fakeWrite(filepath.Join(root, "data"), "retained")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if digest, err := bundleDigestContext(ctx, root, 1024); err == nil || digest != "" {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
	reader := &contextReader{ctx: ctx, reader: strings.NewReader("data")}
	if n, err := reader.Read(make([]byte, 4)); n != 0 || err == nil {
		t.Fatalf("canceled streaming read=%d err=%v", n, err)
	}
}

func TestObservedNativeTimeoutBasenameIsNotPathAuthority(t *testing.T) {
	t.Parallel()
	trace := filepath.Join(t.TempDir(), "capture.trace")
	native := "Reached specified time limit, ending recording...\nRecording completed. Saving output file...\nOutput file saved as: capture.trace\n"
	if !timeoutMarkers(native, trace) {
		t.Fatal("rejected observed native spelling for fixed bundle")
	}
	for _, tc := range []struct{ name, output, path string }{
		{"other fixed filename", native, filepath.Join(filepath.Dir(trace), "other.trace")},
		{"arbitrary basename", strings.Replace(native, "capture.trace", "other.trace", 1), trace},
		{"basename without native limit", strings.Replace(native, "Reached specified time limit, ending recording...", "Reached specified time limit", 1), trace},
		{"basename without native completion", strings.Replace(native, "Recording completed. Saving output file...", "Recording completed", 1), trace},
		{"conflicting absolute save", native + "Output file saved as: " + trace + ".other\n", trace},
		{"extra valid absolute save", native + "Output file saved as: " + trace + "\n", trace},
		{"out of order", "Recording completed. Saving output file...\nReached specified time limit, ending recording...\nOutput file saved as: capture.trace\n", trace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if timeoutMarkers(tc.output, tc.path) {
				t.Fatal("accepted unsupported or ambiguous native marker binding")
			}
		})
	}
}
