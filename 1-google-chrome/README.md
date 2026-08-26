# google-chrome

[Google Chrome](https://www.google.com/chrome/) (stable) on
[`ghcr.io/cnuss/ubuntu:24.04`](../0-ubuntu), running as the non-root `user`
account (uid 1000). Multi-arch: `linux/amd64` + `linux/arm64` (Google ships
native Linux arm64 Chrome since July 2026).

Tags track the upstream Chrome stable version — `:latest` is rebuilt nightly.

## Usage

The default entrypoint launches Chrome inside an [xpra](https://xpra.org)
session and serves it to your browser via xpra's HTML5 client on port 14500,
with the Chrome DevTools Protocol listening on port 9222:

```sh
docker run --rm -p 14500:14500 ghcr.io/cnuss/google-chrome
```

Then open <http://localhost:14500>. Arguments pass through to Chrome
(arguments containing spaces are not supported by the wrapper):

```sh
docker run --rm -p 14500:14500 ghcr.io/cnuss/google-chrome https://example.com
```

The xpra session has **no authentication** — anyone who can reach the port
sees and controls the browser. Authentication/authorization is expected to be
handled by whatever layer exposes the port (reverse proxy, tunnel, VPN); keep
it on localhost otherwise. Multiple clients may connect at once
(`--sharing=yes`) and all see the same session. The container exits when
Chrome exits.

### DevTools protocol (CDP)

**Off by default**: Chrome (136+) disables remote debugging on its default
profile directory. The `--remote-debugging-port=9222` flag is baked in and
activates when you point Chrome at a non-default profile:

```sh
docker run --rm -p 14500:14500 ghcr.io/cnuss/google-chrome --user-data-dir=/tmp/profile
```

CDP then listens on `127.0.0.1:9222` **inside the container's network
namespace** — Chrome binds loopback only, so publishing the port with
`-p 9222:9222` does not reach it. Connect from inside the namespace:

```sh
docker exec <container> curl -s http://127.0.0.1:9222/json/version
```

or share the namespace (`docker run --network container:<container> …`,
`--network host` on Linux, or a sidecar in the same pod).

### Headless

Bypass xpra with `--entrypoint google-chrome`. Headless screenshot of a page:

```sh
docker run --rm --entrypoint google-chrome -v "$PWD":/home/user ghcr.io/cnuss/google-chrome \
  --headless --no-sandbox --disable-gpu --screenshot=/home/user/shot.png \
  https://example.com
```

`--no-sandbox` is required under Docker's default seccomp profile (the xpra
entrypoint adds it automatically). To keep the browser sandbox instead, run the
container with a seccomp profile that allows user namespaces (or
`--cap-add=SYS_ADMIN`) and drop the flag.

## Build locally

Build the base image first, then this one:

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/
docker build -t google-chrome \
  --build-arg VERSION="$(./1-google-chrome/version.sh)" 1-google-chrome/
```
