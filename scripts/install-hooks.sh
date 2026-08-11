#!/usr/bin/env bash
# Point git at the repo's tracked hooks so the pre-push CI gate runs locally.
# Run once per clone:  scripts/install-hooks.sh
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git config core.hooksPath .githooks
chmod +x .githooks/* 2>/dev/null || true
echo "core.hooksPath -> .githooks (pre-push CI gate active). Bypass once with: git push --no-verify"
