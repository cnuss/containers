#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as.
# Pinned to the 6.18 LTS kernel: a new mainline release can break the UML
# static build, so moving to one is a deliberate bump, not a nightly surprise.
set -euo pipefail
echo "6.18"
