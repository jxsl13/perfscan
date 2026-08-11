#!/usr/bin/env bash
# Generate a machine-local compile_commands.json for sample.cpp.
set -euo pipefail
cd "$(dirname "$0")"
CXX="$(brew --prefix llvm 2>/dev/null)/bin/clang++"; [ -x "$CXX" ] || CXX=clang++
SDK="$(xcrun --show-sdk-path 2>/dev/null || true)"
ISYS=""; [ -n "$SDK" ] && ISYS="-isysroot $SDK"
cat > compile_commands.json <<JSON
[
  { "directory": "$(pwd)", "command": "$CXX -std=c++17 $ISYS -c sample.cpp", "file": "$(pwd)/sample.cpp" }
]
JSON
echo "wrote $(pwd)/compile_commands.json"
