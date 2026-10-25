#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as.
set -euo pipefail
curl -fsSL https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/linux_amd64_musl.json | jq -r .version
