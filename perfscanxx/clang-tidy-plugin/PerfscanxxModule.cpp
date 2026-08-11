//===--- PerfscanxxModule.cpp - perfscanxx-tidy ---------------------------===//
//
// Registers the perfscan++ clang-tidy module. Built as a shared library and
// loaded at runtime by clang-tidy (>= 15) via `clang-tidy --load=<lib>`; the
// perfscanxx CLI passes that flag automatically when a plugin path is
// configured (see plugin README). The same file also works compiled in-tree
// into clang-tidy — then the anchor variable at the bottom must be referenced
// from ClangTidyMain.cpp / ClangTidyForceLinker.h so the linker keeps the
// static registry entry.
//
// SPDX-License-Identifier: MIT
//
//===----------------------------------------------------------------------===//

#include "clang-tidy/ClangTidyModule.h"
#include "clang-tidy/ClangTidyModuleRegistry.h"

#include "ReserveBeforeLoopCheck.h"

namespace clang::tidy {
namespace perfscanxx {

/// All perfscan++ custom checks live in the "perfscanxx-" namespace so that
/// `--checks=perfscanxx-*` selects exactly this catalog and the perfscanxx
/// orchestrator can probe for plugin availability via `--list-checks`.
class PerfscanxxModule : public ClangTidyModule {
public:
  void addCheckFactories(ClangTidyCheckFactories &CheckFactories) override {
    CheckFactories.registerCheck<ReserveBeforeLoopCheck>(
        "perfscanxx-reserve-before-loop");
    // Future perfscan analogs register here, e.g.:
    //   perfscanxx-string-concat-in-loop     (PS2103 analog)
    //   perfscanxx-repeated-map-lookup       (PS2104 analog)
  }
};

} // namespace perfscanxx

// Static-registry entry. When the shared library is dlopen'ed by
// `clang-tidy --load`, this global's constructor runs and the module becomes
// visible exactly as if it were built in.
static ClangTidyModuleRegistry::Add<perfscanxx::PerfscanxxModule>
    X("perfscanxx-module", "Adds perfscan++ performance checks.");

// In-tree linking only: anchor so --gc-sections / lazy archive semantics do
// not drop the registration object file. Unused (but harmless) in the
// --load plugin build.
volatile int PerfscanxxModuleAnchorSource = 0;

} // namespace clang::tidy
