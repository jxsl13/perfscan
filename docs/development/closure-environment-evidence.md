# PS6126 compiler evidence workflow

`perfscan-closureenv` compiles the selected package in two complete module
checkouts and emits the paired evidence consumed by PS6126. Use the same Go
binary, target, tags and environment for both revisions. When edits move the
closure, provide distinct sites:

```sh
CGO_ENABLED=0 GOEXPERIMENT=simd go run ./cmd/perfscan-closureenv \
  -before-dir /src/before -after-dir /src/after -package ./backend/cpu \
  -function gemmF32AMXCompute \
  -before-file gemm_amx_arm64.go -before-line 112 -before-column 55 \
  -after-file gemm_amx_arm64.go -after-line 114 -after-column 55 \
  -go /path/to/go -goos darwin -goarch arm64 > closure-growth.json
```

Package mode uses the selected `go` command for imports, module language,
build tags and compiler invocation; records GOFLAGS, GOEXPERIMENT, CGO and
architecture tuning; hashes the complete selected package and dependency source
context around compilation/type loading; and writes build output to a temporary
file. Bare-file mode is only for import-free hermetic evidence and rejects
GOFLAGS it cannot reproduce.

For custom build tags, set `GOFLAGS=-tags=yourtag` in the environment of both
collection and scanning. Explicit collector `-tags` is supported for standalone
layout investigation, but PS6126 rejects such artifact-only tag selection: JSON
must not switch the compiler to a different source universe from the scan.
Likewise select the same `GOOS`, `GOARCH`, `CGO_ENABLED`, `GOEXPERIMENT`, and Go
command in the scanning environment as in the collection command above.

The analyzer recompiles the current package and requires exact equality with
the artifact's current compiler evidence, then joins its file digest and source
position to a typed callback argument of a configured `fanOutHelpers` entry.
Artifact fields are never copied into the command environment: the user's
selected Go environment must already match the recorded flags, experiment, CGO
mode and architecture level, or validation stops before compilation. This
prevents evidence JSON from injecting `-toolexec`, overlays, PGO profiles, or
alternate module files.
Invalid, stale, unsupported, ambiguous or unreproducible evidence fails the
scan. The retained baseline half is checked for internally consistent layout,
scan classification and matching-GOROOT allocation class, but is not
cryptographically authenticated after the baseline checkout is removed.

The compiler descriptor and allocator class are not measurements. Always
measure both B/op and allocs/op at an unchanged full-operation boundary. The
hermetic benchmark demonstrates 224 to 240 B/op with one allocation in both
arms; it does not reproduce GoAI's private 272 to 288 B/op full-operation
campaign or establish a speedup.
