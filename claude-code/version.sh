#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as.
set -euo pipefail
npm view @anthropic-ai/claude-code version
