# zsh

[Zsh](https://www.zsh.org/) with [Oh My Zsh](https://ohmyz.sh/) on
[`ghcr.io/cnuss/ubuntu:24.04`](../0-ubuntu). Runs as the non-root `user`
account (uid 1000). Multi-arch: `linux/amd64` + `linux/arm64`.

Oh My Zsh is installed at `/opt/oh-my-zsh` rather than under the home
directory, so a volume mounted at `/home/user` can't shadow it. It is owned by
`user`, so `omz update` works; the in-shell update prompt is disabled because
the image is rebuilt nightly.

`~/.zshrc` is baked into the image with the `robbyrussell` theme and the `git`
plugin. Mount a volume at `/home/user` and your edits to it — plus shell
history in `~/.zsh_history` — persist between runs.

Oh My Zsh has no upstream releases, so tags are the build date
(`:20260908` style) plus `:latest`, rebuilt nightly.

## Usage

```sh
docker run -it --rm ghcr.io/cnuss/zsh
```

With the current directory as the home directory:

```sh
docker run -it --rm -v "$PWD":/home/user ghcr.io/cnuss/zsh
```

The entrypoint is `zsh`, so arguments pass straight through:

```sh
docker run --rm ghcr.io/cnuss/zsh -c 'echo $ZSH_VERSION'
```

## Build locally

Build the base image first, then this one:

```sh
docker build -t ghcr.io/cnuss/ubuntu:24.04 0-ubuntu/
docker build -t zsh-test 1-zsh/
```
