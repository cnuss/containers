# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this repo is

Multi-arch (`linux/amd64` + `linux/arm64`) container images, built by GitHub Actions
and published to GHCR under `ghcr.io/cnuss/<name>`. Each top-level directory with a
`Dockerfile` is one image. A leading `<digits>-` prefix on the directory sets its
build **tier** and is stripped from the image name (`0-ubuntu` → `ubuntu`,
`1-claude-code` → `claude-code`); unprefixed dirs are tier 0. Adding an image
requires zero workflow edits.

Images:

- `0-ubuntu` → `ghcr.io/cnuss/ubuntu:24.04` — shared base: Ubuntu 24.04 + common
  tools + NodeSource apt repo (no Node installed), flattened to a single layer.
- `1-claude-code` → `ghcr.io/cnuss/claude-code` — Claude Code CLI, `FROM
  ghcr.io/cnuss/ubuntu:24.04`.

## Build pipeline

`build.yml` stages:

1. **discover** — scans top-level dirs for a `Dockerfile`; parses the tier from
   the dir-name prefix; runs each dir's executable `version.sh` to resolve the
   upstream version (falls back to `date +%Y%m%d`); emits per-tier matrix JSON
   `[{tier, name, dir, version}]` as outputs `tier0` and `tier1` (tier ≥ 1 all
   lands in `tier1`). Version is resolved once here so both arch builds pin the
   same version.
2. **tier0**, then **tier1** — each calls the reusable `build-tier.yml`, which
   runs the build/merge pair for its slice of the matrix. `tier1` waits for
   `tier0` so higher tiers build against the freshly published lower-tier
   images (it still runs when tier0 is skipped-empty, but not when it failed).

`build-tier.yml` jobs:

- **build** — matrix of container × arch. Native runners (`ubuntu-24.04` for
  amd64, `ubuntu-24.04-arm` for arm64), no QEMU. Pushes by digest
  (`push-by-digest=true`), uploads digest as artifact. The resolved version is
  passed as the `VERSION` build arg.
- **merge** — downloads digests per container, `docker buildx imagetools create`
  stitches them into one manifest list tagged `:<version>` and `:latest`.

Triggers: push to `main`, PRs (build only, no push — higher tiers then build
against the last *published* base, not the PR's), `workflow_dispatch`, nightly
cron `17 4 * * *` (picks up new upstream releases and base image updates — same
version gets re-tagged/rebuilt, which is intentional).

Auth is plain `GITHUB_TOKEN` with `packages: write`. No repo secrets.

## Design decisions (settled — don't relitigate)

- Registry: **ghcr.io** (native `GITHUB_TOKEN` auth).
- Base: **ubuntu:24.04** (chosen over node-slim/alpine).
- Matrix: **containers × arch on native runners** with digest push + manifest
  merge, rather than single-job QEMU multi-platform build.
- Tags: **track upstream version** (`:2.1.246` style) + `:latest`. No git-tag
  releases, no sha tags.
- Layering for dedup: `0-ubuntu` is scratch-flattened to a **single shared
  layer**; downstream images build on it **without** flattening (flattening a
  leaf image would destroy the layer sharing). BuildKit adds an unavoidable
  empty layer for `WORKDIR` — harmless.
- License: MIT.

## Image conventions

- `0-ubuntu`: stock `ubuntu` user (uid 1000) is deleted and replaced by `user`
  (uid 1000, home `/home/user`) so container files map cleanly onto host volume
  mounts; `/workspace` exists and is `user`-owned. Default user stays root
  (like upstream ubuntu) — leaf images opt in with `USER user`. `ENV`/`USER`/
  `WORKDIR` config does not survive the scratch-flatten, so leaf images set
  their own (including `DEBIAN_FRONTEND` via `ARG` for apt runs).
- `1-claude-code`: `ARG VERSION` pins `@anthropic-ai/claude-code@${VERSION}` —
  keeps both arches identical. Node 22 from the base's preconfigured NodeSource
  repo. Entrypoint `claude`, workdir `/workspace`. Users mount their project at
  `/workspace` and `~/.claude` at `/home/user/.claude`.

## Verifying changes locally

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/          # base first (local tag shadows registry)
docker build -t claude-code-test \
  --build-arg VERSION="$(./1-claude-code/version.sh)" 1-claude-code/
docker run --rm claude-code-test --version                    # expect "<version> (Claude Code)"
docker run --rm --entrypoint id claude-code-test              # expect uid=1000(user)
```

CI watch: `gh run watch -R cnuss/containers --exit-status <run-id>`.

## State as of 2026-08-26

- Published (pre-restructure): `ghcr.io/cnuss/claude-code` `:2.1.246` +
  `:latest`. Repo and GHCR package are public.
- **Uncommitted WIP**: tier restructure — `claude-code/` → `1-claude-code/`,
  new `0-ubuntu/` base image, workflow split into `build.yml` (discover +
  tier sequencing) and reusable `build-tier.yml`. Container user renamed
  `claude` → `user`. Built and verified locally: base layer digest is shared
  between the two images; version/uid/workspace/node checks pass. Not yet
  committed or run in CI.

## Known follow-ups

- CI annotations warn upstream actions (`actions/checkout@v4`,
  `docker/build-push-action@v6`, etc.) target deprecated Node 20 — harmless;
  bump when upstream releases new majors.
- New GHCR packages are private by default and there is no API for visibility —
  after `ubuntu` first publishes, flip it manually at
  <https://github.com/users/cnuss/packages/container/ubuntu/settings>.
