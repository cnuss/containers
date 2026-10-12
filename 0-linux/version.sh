#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as:
# the newest release in the 6.18 LTS series, from the stable tree's tags.
# Moving to a newer series is a deliberate bump, not a nightly surprise: a
# new mainline release can break the UML static build.
set -euo pipefail
series=6.18
git ls-remote --tags --refs \
    https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git "v${series}*" \
    | sed 's|.*refs/tags/v||' \
    | grep -E "^${series//./\\.}(\\.[0-9]+)?\$" \
    | sort -V \
    | tail -n 1
