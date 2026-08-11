# perfscanxx custom checks — query-based, ZERO compiled C++

perfscan++ deliberately contains **as little C/C++ as possible**. Its custom
performance checks (the ones clang-tidy lacks) are NOT a compiled clang-tidy
plugin — they are declarative **clang-query matcher strings** in a config file,
run by clang-tidy's experimental query-based custom-check engine (LLVM ≥ 20):

    clang-tidy --experimental-custom-checks \
      --config-file=perfscanxx-custom.clang-tidy \
      -p <build> file.cpp

The only C++ in the whole tool is the prebuilt `clang-tidy` binary (a runtime
dependency). Everything perfscanxx ships is Go + matcher strings.

`perfscanxx-custom.clang-tidy` currently defines `reserve-before-loop` (the C++
analog of the Go linter's PS2101): a `push_back`/`emplace_back` inside a loop
with no prior `reserve()`. Add more checks by appending `CustomChecks` entries
— each is a `match ...` clang-query plus a message; no build step.

TODO (Go wiring): teach the orchestrator to merge these into the generated
`.clang-tidy` and pass `--experimental-custom-checks` automatically, so
`perfscanxx -checks custom-*` just works.

See https://clang.llvm.org/extra/clang-tidy/QueryBasedCustomChecks.html
