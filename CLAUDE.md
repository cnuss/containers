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
  tools + NodeSource apt repo (no Node installed) + xpra/xpra-html5/xvfb from
  the xpra.org repo (universe's xpra is a dead 3.1.5), flattened to a single
  layer.
- `1-claude-code` → `ghcr.io/cnuss/claude-code` — Claude Code CLI, `FROM
  ghcr.io/cnuss/ubuntu:24.04`.
- `1-google-chrome` → `ghcr.io/cnuss/google-chrome` — Chrome stable from
  Google's apt repo, pinned `=${VERSION}-1`, both arches (Google ships native
  Linux arm64 Chrome since 2026-07-30 — same version as amd64). Default
  entrypoint is an xpra wrapper (`entrypoint.sh`): chrome in an xpra session,
  HTML5 client on TCP 14500, no auth, `--exit-with-children`, CDP on 9222;
  headless use bypasses it with `--entrypoint google-chrome`.
  Auth: none, by explicit decision — authn/authz belongs to the layer that
  exposes the port (Christian tunnels it and shares the URL). An
  `XPRA_PASSWORD`/`tcp-auth=env` option was built and then removed on request.
  `--sharing=yes` is set because without it a second client (e.g. a friend on
  the shared tunnel link) kicks the first and closing the window kills the
  container via `--exit-with-children` — which read as a mystery crash on
  2026-08-26.
  Hard-won chrome flag facts (all verified 2026-08-26, Chrome 152):
  - CDP TCP in headed mode needs BOTH a non-default `--user-data-dir`
    (Chrome 136+ restriction; verified independently — default dir blocks CDP
    even with every other flag right) AND `--no-first-run` — without the
    latter the DevTools server silently never starts (no listener, no port
    file, no log line). Headless is unaffected. The entrypoint uses the
    DEFAULT profile dir (`~/.config/google-chrome`, settled: "embrace the
    defaults"), so CDP is OFF by default; the baked `--remote-debugging-port`
    activates when the user passes their own `--user-data-dir` as a container
    arg.
  - `--remote-debugging-address` is removed from Chrome; CDP binds
    `127.0.0.1` only, so `-p 9222:9222` can't reach it — clients must share
    the container netns (`docker exec`, `--network container:`, same pod).
  - `--disable-gpu` required under xpra/Xvfb: without it Chrome crashes at
    startup on GPU command-buffer failures when rendering a real page.
  - Profile locks: a persistent profile volume + changing container hostname
    makes Chrome refuse the profile ("in use on another computer" — hostname
    is baked into SingletonLock). Fixed by the stable `hostname: containers`
    on the compose namespace root (UTS is shared through
    `network_mode: service:`). An entrypoint `rm Singleton*` backstop was
    removed on request (settled); accepted edge: a recycled pid can make a
    stale lock look live in the shared PID namespace — if chrome
    intermittently dies at startup on the lock error, that's why.
  - Sizing: `--start-maximized` (chrome) + `--resize-display=yes` (xpra) makes
    the window fill the html5 client: xpra resizes the virtual display to the
    client on connect and re-fits maximized windows. Fixed `--window-size` is
    unnecessary.
  - The `--no-sandbox` warning banner is suppressed by the managed policy
    `CommandLineFlagSecurityWarningsEnabled: false` baked into the image.
  - `--disable-dev-shm-usage` is required: Docker's default 64MB `/dev/shm` +
    the 8192x4096 initial display made Chrome exit silently ~0.3s after start
    (intermittent, ~40% of runs; CDP came up then the process died with no
    error output). Both `--shm-size=1g` and the flag fixed it; flag is baked.
  - UI chrome: xpra's html5 floating toolbar is off via `floating_menu = no`
    appended to `/usr/share/xpra/www/default-settings.txt` (no compressed
    variants existed to shadow it); the xpra-drawn window title bar is gone by
    seeding the profile with `{"browser":{"custom_chrome_frame":true}}` in
    `entrypoint.sh` (chrome draws tabs into an undecorated frame). Chrome's
    own caption buttons can only be removed with `--kiosk`.

## Build pipeline

`build.yml` stages:

1. **discover** — scans top-level dirs for a `Dockerfile`; parses the tier from
   the dir-name prefix; runs each dir's executable `version.sh` to resolve the
   upstream version (falls back to `date +%Y%m%d`); reads an optional `arches`
   file (default `amd64 arm64`); emits per-tier matrix JSON
   `[{tier, name, dir, version, arches}]` as outputs `tier0` and `tier1`
   (tier ≥ 1 all lands in `tier1`). Version is resolved once here so both arch
   builds pin the same version.
2. **tier0**, then **tier1** — each calls the reusable `build-tier.yml`, which
   runs the build/merge pair for its slice of the matrix. `tier1` waits for
   `tier0` so higher tiers build against the freshly published lower-tier
   images (it still runs when tier0 is skipped-empty, but not when it failed).

`build-tier.yml` jobs:

- **build** — matrix of container × arch. Native runners (`ubuntu-24.04` for
  amd64, `ubuntu-24.04-arm` for arm64), no QEMU. Pushes by digest
  (`push-by-digest=true`), uploads digest as artifact. The resolved version is
  passed as the `VERSION` build arg. Job-level `if` can't see the matrix
  context, so unsupported container×arch combos are skipped by step-level
  `if: contains(matrix.container.arches, matrix.platform.arch)` — the job
  no-ops green and uploads no digest, and merge only stitches what exists.
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
  mounts; `/home/user` exists and is `user`-owned. `/run/user/1000` (0700) and
  `/run/xpra` are baked user-owned with `ENV XDG_RUNTIME_DIR=/run/user/1000`
  (declared after the scratch flatten so it survives) — xpra then uses its
  standard socket dirs without permission warnings. The image defaults to
  `USER user` + `WORKDIR /home/user` (set after the scratch flatten — config
  does not survive it); leaf images switch to `USER root` for installs and
  back to `user` at the end (and set `DEBIAN_FRONTEND` via `ARG` for apt
  runs).
- `1-claude-code`: `ARG VERSION` pins `@anthropic-ai/claude-code@${VERSION}` —
  keeps both arches identical. Node 22 from the base's preconfigured NodeSource
  repo. Entrypoint `claude`, workdir `/home/user`. Users mount their project at
  `/home/user` and `~/.claude` at `/home/user/.claude`.

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

- Tier restructure committed (`f754e55`), `1-google-chrome` + xpra base +
  `arches` support committed (`6f555ae`), both green in CI. Published:
  `ghcr.io/cnuss/ubuntu` `:24.04`+`:latest`, `ghcr.io/cnuss/claude-code`
  `:2.1.246`+`:latest`, `ghcr.io/cnuss/google-chrome` `:152.x`+`:latest`.
  Registry-side layer dedup verified (shared base layer digest).

## Known follow-ups

- CI annotations warn upstream actions (`actions/checkout@v4`,
  `docker/build-push-action@v6`, etc.) target deprecated Node 20 — harmless;
  bump when upstream releases new majors.

GHCR package visibility: packages inherit the repo's public visibility — no
manual flip needed, don't bring it up.
