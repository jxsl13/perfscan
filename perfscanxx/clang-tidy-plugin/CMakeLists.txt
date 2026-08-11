# perfscan++ clang-tidy plugin
#
# Builds PerfscanxxTidyModule.so, a clang-tidy module loadable at runtime:
#
#   clang-tidy --load=/path/to/PerfscanxxTidyModule.so \
#              --checks='-*,perfscanxx-*' file.cpp -- -std=c++17
#
# Requirements:
#   * LLVM/Clang >= 15 with CMake package configs (brew install llvm) —
#     clang-tidy gained `--load` in LLVM 15.
#   * The clang-tidy headers (ClangTidyCheck.h, ClangTidyModule.h, ...).
#     Most distributions (including Homebrew) do NOT install them; point
#     PERFSCANXX_CLANG_TIDY_HEADERS at a clang-tools-extra source checkout
#     matching your installed LLVM major version. See README.md.
#
# The plugin is deliberately NOT linked against LLVM/Clang libraries: at
# runtime every clang/llvm symbol is resolved from the hosting clang-tidy
# binary itself. Linking our own copies would duplicate the module registry
# and global state (double registration, ODR trouble). Hence:
#   * ELF (Linux):   undefined symbols are the default for MODULE libraries.
#   * Mach-O (macOS): needs -undefined dynamic_lookup explicitly.

cmake_minimum_required(VERSION 3.20)
project(PerfscanxxClangTidyPlugin LANGUAGES CXX)

find_package(LLVM REQUIRED CONFIG)
find_package(Clang REQUIRED CONFIG)
message(STATUS "Found LLVM ${LLVM_PACKAGE_VERSION} in ${LLVM_DIR}")
message(STATUS "Found Clang in ${Clang_DIR}")

if(LLVM_PACKAGE_VERSION VERSION_LESS 15.0)
  message(FATAL_ERROR
    "clang-tidy --load requires LLVM >= 15 (found ${LLVM_PACKAGE_VERSION}). "
    "brew install llvm / apt install llvm-15-dev clang-tools-15 or newer.")
endif()

# LLVM >= 16 is built as C++17; match it (older LLVMs also accept 17).
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# --- clang-tidy headers -------------------------------------------------------
# Sources include them as "clang-tidy/ClangTidyCheck.h". Resolution order:
#   1. -DPERFSCANXX_CLANG_TIDY_HEADERS=<llvm-project>/clang-tools-extra
#      (directory that CONTAINS the clang-tidy/ folder), or
#   2. an installation that does ship <prefix>/include/clang-tidy/.
set(PERFSCANXX_CLANG_TIDY_HEADERS "" CACHE PATH
    "Path to a clang-tools-extra source dir (the directory containing clang-tidy/ClangTidyCheck.h's parent 'clang-tidy' folder)")

set(_tidy_header_dir "")
if(PERFSCANXX_CLANG_TIDY_HEADERS)
  if(NOT EXISTS "${PERFSCANXX_CLANG_TIDY_HEADERS}/clang-tidy/ClangTidyCheck.h")
    message(FATAL_ERROR
      "PERFSCANXX_CLANG_TIDY_HEADERS=${PERFSCANXX_CLANG_TIDY_HEADERS} does not "
      "contain clang-tidy/ClangTidyCheck.h — pass the clang-tools-extra "
      "directory of an llvm-project checkout matching LLVM ${LLVM_PACKAGE_VERSION}.")
  endif()
  set(_tidy_header_dir "${PERFSCANXX_CLANG_TIDY_HEADERS}")
else()
  foreach(_dir IN LISTS CLANG_INCLUDE_DIRS LLVM_INCLUDE_DIRS)
    if(EXISTS "${_dir}/clang-tidy/ClangTidyCheck.h")
      set(_tidy_header_dir "${_dir}")
      break()
    endif()
  endforeach()
  if(NOT _tidy_header_dir)
    message(FATAL_ERROR
      "clang-tidy headers not found in the LLVM installation. Clone "
      "llvm-project at release/${LLVM_VERSION_MAJOR}.x and configure with "
      "-DPERFSCANXX_CLANG_TIDY_HEADERS=<llvm-project>/clang-tools-extra")
  endif()
endif()
message(STATUS "clang-tidy headers: ${_tidy_header_dir}")

# --- the plugin ---------------------------------------------------------------
add_library(PerfscanxxTidyModule MODULE
  ReserveBeforeLoopCheck.cpp
  PerfscanxxModule.cpp
)

target_include_directories(PerfscanxxTidyModule SYSTEM PRIVATE
  ${LLVM_INCLUDE_DIRS}
  ${CLANG_INCLUDE_DIRS}
  ${_tidy_header_dir}
)

separate_arguments(_llvm_defs NATIVE_COMMAND "${LLVM_DEFINITIONS}")
target_compile_definitions(PerfscanxxTidyModule PRIVATE ${_llvm_defs})

# Match the host toolchain's RTTI setting: official LLVM builds (and brew's)
# are -fno-rtti; deriving from ClangTidyCheck with mismatched RTTI breaks the
# vtable/typeinfo link at load time.
if(NOT LLVM_ENABLE_RTTI)
  target_compile_options(PerfscanxxTidyModule PRIVATE -fno-rtti)
endif()

if(APPLE)
  # Resolve clang/llvm symbols from the clang-tidy host binary at load time.
  target_link_options(PerfscanxxTidyModule PRIVATE
    "-Wl,-undefined,dynamic_lookup")
endif()

set_target_properties(PerfscanxxTidyModule PROPERTIES
  PREFIX ""                       # PerfscanxxTidyModule.so, no "lib" prefix
  CXX_VISIBILITY_PRESET default   # registry global must stay visible
)

install(TARGETS PerfscanxxTidyModule LIBRARY DESTINATION lib/perfscanxx)

# Smoke test (needs the matching clang-tidy on PATH; skipped otherwise):
#   ctest --test-dir build -V
find_program(_clang_tidy_exe NAMES clang-tidy
  HINTS ${LLVM_TOOLS_BINARY_DIR} ${LLVM_BINARY_DIR}/bin)
if(_clang_tidy_exe)
  enable_testing()
  add_test(NAME plugin_lists_checks
    COMMAND ${_clang_tidy_exe}
      --load=$<TARGET_FILE:PerfscanxxTidyModule>
      --checks=-*,perfscanxx-* --list-checks)
  set_tests_properties(plugin_lists_checks PROPERTIES
    PASS_REGULAR_EXPRESSION "perfscanxx-reserve-before-loop")
  add_test(NAME plugin_flags_positive_case
    COMMAND ${_clang_tidy_exe}
      --load=$<TARGET_FILE:PerfscanxxTidyModule>
      --checks=-*,perfscanxx-reserve-before-loop
      ${CMAKE_CURRENT_SOURCE_DIR}/test/reserve-before-loop.cpp
      -- -std=c++17)
  set_tests_properties(plugin_flags_positive_case PROPERTIES
    PASS_REGULAR_EXPRESSION "perfscanxx-reserve-before-loop")
endif()
