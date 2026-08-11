# perfscanxx validation on real complex C++ codebases

perfscanxx (Go orchestrator over clang-tidy, zero of our own C++) run against
complex real-world C++ as test data. All configured with the brew clang++
toolchain (so headers match clang-tidy 22); DDNet additionally needed its
codegen targets built for generated/*.h.

| Codebase | TUs analyzed | Load errors | PX findings | Breakdown | Fixable |
|----------|-------------:|------------:|------------:|-----------|--------:|
| fmt      | 31 (all)     | 0 | 26 | PX1002:15 PX3003:10 PX2002:1 | 25 |
| spdlog   | 34 (all own) | 0 |  6 | PX1002:3 PX3001:2 PX2001:1   | 4  |
| leveldb  | 39 (all)     | 0 |  1 | PX1002:1                     | 1  |
| abseil   | 159 (all)    | 0 |  4 | PX1002:2 PX3002:1 PX3003:1   | 4  |
| DDNet    | ~40 sample   | (needs codegen) | 1+ | PX1002 (std::function by value) | yes |

Findings are real and low-noise: mature library *sources* are largely clean
(most fmt/spdlog findings are in test/vendored-gtest code), while perfscanxx
still surfaces genuine copy/move/endl pitfalls with fix-its. leveldb was
cross-validated against raw clang-tidy restricted to the same 8 mapped
checks — identical single finding, confirming the low count is real, not a
tool miss. Every codebase configured on the first try with no source patches
(except bumping DDNet's throwaway Rust version gate).

Recipe: examples/ddnet-recipe.md. Corpora live under corpus/ (gitignored).
