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
