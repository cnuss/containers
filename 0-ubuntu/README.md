# ubuntu

Base image for the other images in this repo: Ubuntu 24.04 plus `ca-certificates`,
`curl`, `git`, `gnupg`, `less`, `ripgrep`, and `zsh`, with the [NodeSource](https://github.com/nodesource/distributions)
apt repo configured (Node itself is **not** installed), and [xpra](https://xpra.org)
(+ HTML5 client and `xvfb`, from the xpra.org repo) for serving GUI apps to a
browser. Flattened to a single layer so downstream images share it. Multi-arch:
`linux/amd64` + `linux/arm64`.

The stock `ubuntu` user is replaced by `user` (uid 1000, home `/home/user`) so
container files map cleanly onto host volume mounts, and the image runs as
that user by default (`WORKDIR /home/user`). Downstream images switch to
`USER root` for their installs and back.

## Usage

```sh
docker run -it --rm ghcr.io/cnuss/ubuntu:24.04
```

As a base image:

```dockerfile
FROM ghcr.io/cnuss/ubuntu:24.04

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

USER user
```

## Build locally

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/
```
