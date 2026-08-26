#!/usr/bin/env bash
# Prints the upstream version this container should be built and tagged as.
set -euo pipefail
curl -fsSL 'https://chromiumdash.appspot.com/fetch_releases?channel=Stable&platform=Linux&num=1' \
  | jq -r '.[0].version'
