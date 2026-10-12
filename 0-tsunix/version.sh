#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as:
# the newest stable Tailscale release (even minor versions are stable).
set -euo pipefail
git ls-remote --tags --refs https://github.com/tailscale/tailscale 'v1.*' \
    | sed 's|.*refs/tags/v||' \
    | grep -E '^1\.[0-9]*[02468]\.[0-9]+$' \
    | sort -V \
    | tail -n 1
