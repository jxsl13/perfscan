package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeSchemas(t *testing.T) {
	t.Parallel()
	value, err := decodeSchemas([]byte(`[{"columns":["timestamp","value"],"name":"gpu-counter-value"}]`))
	if err != nil || len(value) != 1 || value[0].Name != "gpu-counter-value" || len(value[0].Columns) != 2 {
		t.Fatalf("schemas=%+v err=%v", value, err)
	}
	for _, input := range []string{
		``, `null`, `[]`, `{}`, `[null]`, `[{}]`,
		`[{"name":"s"}]`, `[{"columns":["c"]}]`,
		`[{"name":"s","columns":null}]`, `[{"name":null,"columns":["c"]}]`,
		`[{"name":"s","columns":[null]}]`, `[{"name":"s","columns":[1]}]`,
		`[{"name":"s","columns":["c"],"name":"other"}]`,
		`[{"name":"s","columns":["c"],"extra":true}]`,
		`[{"name":"s","columns":["c"]}] []`,
		`[{"name":"s","columns":["c"]`,
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			if _, err := decodeSchemas([]byte(input)); err == nil {
				t.Fatal("accepted invalid schema configuration")
			}
		})
	}
}

func TestReadSchemaFile(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if _, err := readSchemaFile(directory); err == nil {
		t.Fatal("accepted non-regular configuration")
	}
	path := filepath.Join(directory, "schema.json")
	data := []byte(`[{"name":"example","columns":["value"]}]`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if actual, err := readSchemaFile(path); err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("config read=%s err=%v", actual, err)
	}
	if err := os.WriteFile(path, make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSchemaFile(path); err == nil {
		t.Fatal("accepted oversized configuration")
	}
}

func TestRunHelpAndInvalidOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{{"help", []string{"-help"}, 0}, {"unknown flag", []string{"-unknown"}, 2}, {"no schema file", nil, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if status := run(tc.args, &stdout, &stderr); status != tc.want {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stderr.Len() == 0 || stdout.Len() != 0 {
				t.Fatal("help/error output missing or misplaced")
			}
		})
	}
}
