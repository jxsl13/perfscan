# perfscanxx examples

`sample.cpp` contains deliberate C++ performance anti-patterns (a range-for
that copies a large struct, an expensive by-value parameter). Use it to smoke
perfscanxx end-to-end.

clang-tidy needs a compile command per translation unit. `compile_commands.json`
is machine-specific (absolute paths + macOS SDK sysroot), so it is generated,
not committed:

    ./gen-compile-db.sh          # writes compile_commands.json for this machine

Then run perfscanxx (keg-only brew llvm is not on PATH):

    PERFSCANXX_CLANG_TIDY="$(brew --prefix llvm)/bin/clang-tidy" \
      go run ../cmd/perfscanxx -p . sample.cpp          # report
    PERFSCANXX_CLANG_TIDY=... go run ../cmd/perfscanxx -fix -p . sample.cpp
