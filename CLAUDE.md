# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this repo is

Multi-arch (`linux/amd64` + `linux/arm64`) container images, built by GitHub Actions
and published to GHCR under `ghcr.io/cnuss/<name>`. Each top-level directory with a
`Dockerfile` is one image; the workflow discovers them automatically — adding an
image requires zero workflow edits.

First image: `claude-code` (Claude Code CLI on Ubuntu 24.04).

## Build pipeline (`.github/workflows/build.yml`)

Three stages:

1. **discover** — scans top-level dirs for a `Dockerfile`; runs each dir's
   executable `version.sh` to resolve the upstream version (falls back to
   `date +%Y%m%d`); emits matrix JSON `[{name, version}]`. Version is resolved
   once here so both arch builds pin the same version.
2. **build** — matrix of container × arch. Native runners (`ubuntu-24.04` for
   amd64, `ubuntu-24.04-arm` for arm64), no QEMU. Pushes by digest
   (`push-by-digest=true`), uploads digest as artifact. The resolved version is
   passed to the build as the `VERSION` build arg.
3. **merge** — downloads digests per container, `docker buildx imagetools create`
   stitches them into one manifest list tagged `:<version>` and `:latest`.

Triggers: push to `main`, PRs (build only, no push), `workflow_dispatch`, nightly
cron `17 4 * * *` (picks up new upstream releases and base image updates — same
version gets re-tagged/rebuilt, which is intentional).

Auth is plain `GITHUB_TOKEN` with `packages: write`. No repo secrets.

## Design decisions (settled — don't relitigate)

- Registry: **ghcr.io** (native `GITHUB_TOKEN` auth).
- `claude-code` base: **ubuntu:24.04** (chosen over node-slim/alpine).
- Matrix: **containers × arch on native runners** with digest push + manifest
  merge, rather than single-job QEMU multi-platform build.
- Tags: **track upstream version** (`:2.1.246` style) + `:latest`. No git-tag
  releases, no sha tags.
- License: MIT.

## claude-code image conventions

- `ARG VERSION` pins `@anthropic-ai/claude-code@${VERSION}` — keeps both arches
  identical.
- Node 22 via nodesource apt repo.
- Stock `ubuntu` user (uid 1000) is deleted and replaced by `claude` (uid 1000)
  so container files map cleanly onto host volume mounts.
- Entrypoint `claude`, workdir `/workspace`. Users mount their project at
  `/workspace` and `~/.claude` at `/home/claude/.claude`.

## Verifying changes locally

```sh
./claude-code/version.sh                                   # upstream version, e.g. 2.1.246
docker build -t claude-code-test --build-arg VERSION="$(./claude-code/version.sh)" claude-code/
docker run --rm claude-code-test --version                 # expect "<version> (Claude Code)"
docker run --rm --entrypoint id claude-code-test           # expect uid=1000(claude)
```

CI watch: `gh run watch -R cnuss/containers --exit-status <run-id>`.

## State as of 2026-08-26

- Initial commit pushed; first workflow run green end-to-end. Published
  `ghcr.io/cnuss/claude-code` `:2.1.246` + `:latest` (multi-arch manifest).
- **Uncommitted WIP** in `claude-code/Dockerfile`: layer-flatten refactor —
  build stage renamed `combined`, final stage is `FROM scratch` +
  `COPY --from=combined / /`, then `USER`/`WORKDIR`/`ENTRYPOINT`. Squashes image
  to a single layer. Not yet rebuilt/tested locally or in CI.

## Known follow-ups

- GHCR package is private by default — no API for visibility; flip manually at
  <https://github.com/users/cnuss/packages/container/claude-code/settings>.
- CI annotations warn upstream actions (`actions/checkout@v4`,
  `docker/build-push-action@v6`, etc.) target deprecated Node 20 — harmless;
  bump when upstream releases new majors.
