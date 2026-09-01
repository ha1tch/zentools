#!/usr/bin/env bash
# release.sh - thin entry point for zentools releases
#
# All release logic lives in repoman relcore, driven by .repoman.json
# (version_targets, release.steps, release.archive). This script exists
# only as a familiar, discoverable entry point -- `make release` calls
# repoman relcore directly and this wrapper does the same thing, so the
# two can never drift apart the way a second hand-rolled implementation
# would. There used to be one; it's gone now, on purpose.
#
# Usage:
#   ./release.sh <version>            e.g. ./release.sh 0.9.0
#   ./release.sh <version> --resume   resume a release that failed partway
#
# Copyright (c) 2026 haitch
# Licensed under the Apache License, Version 2.0

set -euo pipefail
cd "$(cd "$(dirname "$0")" && pwd)"
exec repoman relcore "$@"
