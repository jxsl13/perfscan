# Testing perfscanxx against DDNet

DDNet (a large real C++ codebase) is used to validate perfscanxx.

    git clone --depth 1 https://github.com/ddnet/ddnet
    cd ddnet

Configure with the SAME toolchain as clang-tidy (brew llvm) so header search
paths match — using Apple's /usr/bin/c++ makes brew clang-tidy fail on libc++:

    cmake -S . -B build -DCMAKE_EXPORT_COMPILE_COMMANDS=ON \
      -DCMAKE_C_COMPILER="$(brew --prefix llvm)/bin/clang" \
      -DCMAKE_CXX_COMPILER="$(brew --prefix llvm)/bin/clang++" \
      -DDOWNLOAD_GTEST=OFF -DPREFER_BUNDLED_LIBS=ON

DDNet generates headers at build time; game/ TUs need them, so build the
codegen targets first (fast, pure Python):

    cmake --build build --target generate_data_types generate_content_types generate_protocol

Then run perfscanxx:

    PERFSCANXX_CLANG_TIDY="$(brew --prefix llvm)/bin/clang-tidy" \
      perfscanxx -p build src/game/client/ui.cpp

Result (real finding): perfscanxx flags `std::function<void()>` parameters
passed by value in src/game/client/ui.h as PX1002 (const-ref), with a fix.
DDNet's src/base is C-style and clean of the curated checks.

## Expected partial parses (codegen-only setup)

The three codegen targets above generate `data_types.h`, `content_types.h` and
`protocol.h`, covering the vast majority of TUs. But ~29 of the ~420 client TUs
(`src/game/client/*.cpp`) also `#include "generated/client_data.h"`, and that
header is produced by `generate_source(... client_content_...)` — a byproduct of a
FULL client build, not one of the named codegen targets (there is no
`generate_client_data` target, and `--target src/generated/client_data.h` is not a
Makefile rule). So a fast, codegen-only setup leaves those ~29 TUs at
`clang-diagnostic-error: 'generated/client_data.h' file not found`.

This is expected and harmless: perfscanxx PARTIALLY analyzes them (up to the missing
include) and summarizes them — `perfscanxx -v` lists exactly which TUs did not fully
parse. To analyze them fully, build enough of the client for `client_data.h` to be
generated (e.g. a full `cmake --build build`); the tradeoff is a much heavier build
for a handful more TUs, so `fetch-corpus.sh` deliberately stops at the fast codegen
targets.
