# containers

Container images built with GitHub Actions and published to GHCR as multi-arch
(`linux/amd64` + `linux/arm64`) manifests, built natively on per-arch runners — no QEMU.

## Images

| Image | Pull | Description |
|---|---|---|
| [ubuntu](./0-ubuntu) | `docker pull ghcr.io/cnuss/ubuntu:24.04` | Ubuntu 24.04 base with common tools, shared by the images below |
| [claude-code](./1-claude-code) | `docker pull ghcr.io/cnuss/claude-code` | [Claude Code](https://claude.com/claude-code) CLI on Ubuntu 24.04 |
| [google-chrome](./1-google-chrome) | `docker pull ghcr.io/cnuss/google-chrome` | [Google Chrome](https://www.google.com/chrome/) stable on Ubuntu 24.04, served via xpra HTML5 |

## Compose

[`docker-compose.yml`](./docker-compose.yml) builds the images from this repo
and runs them in one set of shared namespaces (network, PID, IPC) rooted at
the `zsh` container — chrome and claude-code join its namespaces, so every
service sees the same processes and ports. Chrome's profile
(`/home/user/.config`) and Claude Code's state (`/home/user/.claude`) persist
in named volumes.

```sh
docker compose build zsh && docker compose build   # base first, then leaves
docker compose up -d
docker compose attach zsh           # root shell (detach: ctrl-p ctrl-q)
docker compose attach claude-code   # interactive Claude Code
```

The xpra HTML5 client is at <http://localhost:14500>.

## How it works

Each top-level directory with a `Dockerfile` is one image. A leading `<digits>-`
prefix on the directory name sets its build **tier** and is stripped from the
image name — `0-ubuntu` publishes as `ubuntu`, `1-claude-code` as `claude-code`.
Tier 0 images are built and published before tier 1+, so higher tiers can use
freshly published lower-tier images as their base and share layers with them.

The [build workflow](.github/workflows/build.yml) runs:

1. **discover** — scans the repo for top-level directories containing a
   `Dockerfile`, resolves each image's version (via the directory's
   `version.sh`, falling back to the current date), and splits the matrix into
   tiers.
2. **per tier** (via the [build-tier workflow](.github/workflows/build-tier.yml)):
   - **build** — builds each container on native `amd64` and `arm64` runners in
     parallel, pushing by digest.
   - **merge** — stitches the per-arch digests into one manifest list, tagged
     `:<version>` and `:latest`.

Runs on push to `main`, on a nightly schedule (picks up new upstream releases and
base image updates), and on PRs (build only, no push — PR builds of higher tiers
use the last published lower-tier images).

## Adding an image

1. Create a directory with a `Dockerfile` (and a `README.md`). Prefix the name
   with a tier number if build order matters (`1-foo` builds after `0-ubuntu`);
   unprefixed directories are tier 0.
2. Optionally add an executable `version.sh` that prints the upstream version to
   tag the image with. The version is also passed to the build as the `VERSION`
   build arg.
3. Optionally add an `arches` file listing the supported platforms (e.g. just
   `amd64` for upstreams with no arm64 build). Default is `amd64 arm64`.
4. Push — the workflow discovers it automatically.

## License

[MIT](./LICENSE)
