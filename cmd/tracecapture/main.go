// tracecapture retains artifact-first xctrace evidence, including qualified
// time-limit exits. It is not a benchmark promotion or counter attribution tool.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jxsl13/perfscan/traceevidence"
)

type instruments []string

func (v *instruments) String() string { return strings.Join(*v, ", ") }
func (v *instruments) Set(s string) error {
	*v = append(*v, s)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var o traceevidence.Options
	var selected instruments
	var schemaFile string
	var inputFile string
	flags := flag.NewFlagSet("tracecapture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.Output, "out", "", "NEW retained evidence directory (never overwritten or automatically removed)")
	flags.StringVar(&o.Xcrun, "xcrun", "xcrun", "configured xcrun executable")
	flags.StringVar(&o.Directory, "dir", "", "working directory for the recorder and workload")
	flags.Var(&selected, "instrument", "instrument display name; repeat for every instrument")
	flags.StringVar(&schemaFile, "schemas", "", "JSON array of required schema names and column mnemonics from the selected Instruments version")
	flags.StringVar(&inputFile, "inputs", "", "required audited-complete input inventory JSON; no staging or profiler-child permission guarantee")
	flags.StringVar(&o.Started, "started", "", "exact target-output line marking workload start")
	flags.StringVar(&o.Completed, "completed", "", "exact target-output line attesting successful workload completion")
	flags.DurationVar(&o.TimeLimit, "time-limit", 30*time.Second, "recorder time limit")
	flags.DurationVar(&o.CommandTimeout, "command-timeout", 2*time.Minute, "deadline for each direct recorder/export command, longer than time-limit")
	flags.Int64Var(&o.MaxArtifactBytes, "artifact-bytes", 16<<20, "per stdout/stderr/export/target size limit, at most 64 MiB")
	flags.Int64Var(&o.MaxTraceBytes, "trace-bytes", 1<<30, "post-record trace bundle size limit, at most 8 GiB")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	o.Instruments, o.Workload = selected, flags.Args()
	inputData, err := readSchemaFile(inputFile)
	if err == nil {
		o.InputPolicy, err = decodeInputPolicy(inputData)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "required input inventory:", err)
		return 2
	}
	data, err := readSchemaFile(schemaFile)
	if err == nil {
		o.Schemas, err = decodeSchemas(data)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "required schemas:", err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	result, err := traceevidence.Capture(ctx, &o)
	if result != nil {
		err = errors.Join(err, json.NewEncoder(stdout).Encode(result))
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func readSchemaFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("schema configuration must be a regular file of at most 1 MiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.Join(errors.New("schema configuration changed while opening"), err, f.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	err = errors.Join(readErr, f.Close())
	if len(data) > 1<<20 {
		err = errors.Join(err, errors.New("schema configuration exceeds 1 MiB"))
	}
	return data, err
}

func decodeSchemas(data []byte) ([]traceevidence.Schema, error) {
	d := json.NewDecoder(strings.NewReader(string(data)))
	delim := func(want json.Delim) error {
		token, err := d.Token()
		if err != nil {
			return err
		}
		if token != want {
			return fmt.Errorf("expected schema configuration delimiter %q", want)
		}
		return nil
	}
	if err := delim('['); err != nil {
		return nil, err
	}
	var schemas []traceevidence.Schema
	for d.More() {
		if err := delim('{'); err != nil {
			return nil, err
		}
		var schema traceevidence.Schema
		seen := make(map[string]bool, 2)
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := token.(string)
			if !ok || seen[name] {
				return nil, errors.New("invalid or duplicate schema configuration key")
			}
			seen[name] = true
			switch name {
			case "name":
				err = d.Decode(&schema.Name)
			case "columns":
				err = d.Decode(&schema.Columns)
			default:
				return nil, fmt.Errorf("unknown schema configuration key %q", name)
			}
			if err != nil {
				return nil, err
			}
		}
		if !seen["name"] || !seen["columns"] || schema.Name == "" || len(schema.Columns) == 0 {
			return nil, errors.New("schema name and nonempty required columns are mandatory")
		}
		for _, column := range schema.Columns {
			if column == "" {
				return nil, errors.New("required column cannot be null or empty")
			}
		}
		if err := delim('}'); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	if err := delim(']'); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("trailing schema configuration data")
	}
	if len(schemas) == 0 {
		return nil, errors.New("at least one required schema is mandatory")
	}
	return schemas, nil
}
