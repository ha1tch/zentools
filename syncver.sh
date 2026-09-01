#!/usr/bin/env bash
# syncver.sh - thin entry point for version sync
#
# All version-sync logic lives in repoman syncver, driven by
# .repoman.json's version_file/version_targets. This script is a
# familiar, discoverable entry point with the same subcommands
# (show, set, check, bump-patch, bump-minor, bump-major) -- nothing
# more, so it can't drift out of sync with the real implementation.
#
# Usage:
#   ./syncver.sh show
#   ./syncver.sh set 0.9.0
#   ./syncver.sh check
#   ./syncver.sh bump-patch | bump-minor | bump-major
#
# Copyright (c) 2026 haitch
# Licensed under the Apache License, Version 2.0

set -euo pipefail
cd "$(cd "$(dirname "$0")" && pwd)"
exec repoman syncver "$@"
