#!/usr/bin/env bash
# Prints the version this container should be built and tagged as. LKL has no
# releases: it is the kernel version of lkl/linux's master branch plus the
# commit, e.g. 6.12.0-d0f76a77e8c2.
set -euo pipefail
sha=$(git ls-remote https://github.com/lkl/linux refs/heads/master | cut -f1)
curl -fsSL "https://raw.githubusercontent.com/lkl/linux/${sha}/Makefile" \
    | awk -v sha="${sha:0:12}" '
        /^VERSION =/ { v = $3 }
        /^PATCHLEVEL =/ { p = $3 }
        /^SUBLEVEL =/ { s = $3 }
        END { print v "." p "." s "-" sha }'
