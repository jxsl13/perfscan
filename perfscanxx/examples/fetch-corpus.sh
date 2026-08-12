#!/usr/bin/env bash
# Fetch + configure complex real-world C++ codebases as perfscanxx test data.
#
# Each project is cloned (shallow) under corpus/ (gitignored) and configured with
# CMake + CMAKE_EXPORT_COMPILE_COMMANDS=ON using the brew clang++ toolchain, so its
# build/compile_commands.json matches the clang-tidy that perfscanxx drives. After
# this runs, validate with, e.g.:
#
#     cd corpus/leveldb && perfscanxx -p build -level 3 ./...
#
# These are deliberately COMPLEX, TU-heavy codebases (not toy samples): a header
# library (fmt), a logging lib (spdlog), an embedded KV store (leveldb), a large
# foundational library (abseil, ~490 TUs), and a full game/engine (DDNet, ~420 TUs;
# its codegen targets are built automatically so generated/*.h exist).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)/corpus"
mkdir -p "$ROOT"
CXX="$(brew --prefix llvm 2>/dev/null)/bin/clang++"; [ -x "$CXX" ] || CXX=clang++
CC="$(brew --prefix llvm 2>/dev/null)/bin/clang";   [ -x "$CC" ]  || CC=clang

# name  repo  tag  [extra cmake args…]
projects=(
  "leveldb https://github.com/google/leveldb.git    main  -DLEVELDB_BUILD_TESTS=OFF -DLEVELDB_BUILD_BENCHMARKS=OFF"
  "fmt     https://github.com/fmtlib/fmt.git         master"
  "spdlog  https://github.com/gabime/spdlog.git      v1.x  -DSPDLOG_BUILD_EXAMPLE=ON"
  "abseil  https://github.com/abseil/abseil-cpp.git  master -DABSL_BUILD_TESTING=OFF -DCMAKE_CXX_STANDARD=17"
  # DDNet: a full C++ game/engine (~420 TUs). It generates headers at build time,
  # so configure() also builds its (fast, pure-Python) codegen targets below, so
  # generated/*.h exist for the game/ TUs — see ddnet-recipe.md.
  "ddnet   https://github.com/ddnet/ddnet.git         master -DDOWNLOAD_GTEST=OFF -DPREFER_BUNDLED_LIBS=ON"
)

configure() {
  local name="$1" repo="$2" tag="$3"; shift 3
  local dir="$ROOT/$name"
  if [ ! -d "$dir/.git" ]; then
    echo "== cloning $name ($repo @ $tag)"
    git clone --depth 1 --branch "$tag" "$repo" "$dir"
  else
    echo "== $name already cloned"
  fi
  echo "== configuring $name (compile_commands.json)"
  cmake -S "$dir" -B "$dir/build" \
    -DCMAKE_EXPORT_COMPILE_COMMANDS=ON \
    -DCMAKE_C_COMPILER="$CC" -DCMAKE_CXX_COMPILER="$CXX" \
    "$@" >/dev/null
  local n; n=$(grep -c '"file"' "$dir/build/compile_commands.json" 2>/dev/null || echo 0)
  echo "   -> $dir/build/compile_commands.json ($n translation units)"
  # DDNet generates headers at build time; the game/ TUs #include them, so build
  # the codegen targets (fast, pure Python) so generated/*.h exist before scanning.
  if [ "$name" = ddnet ]; then
    echo "== building DDNet codegen targets (generated/*.h)"
    cmake --build "$dir/build" \
      --target generate_data_types generate_content_types generate_protocol >/dev/null
  fi
}

for p in "${projects[@]}"; do
  # shellcheck disable=SC2086
  configure $p
done

echo
echo "Done. Complex C++ corpora ready under $ROOT. Example run:"
echo "  (cd $ROOT/abseil && perfscanxx -p build -level 3 ./...)"
