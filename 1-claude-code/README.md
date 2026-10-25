# claude-code

The [Claude Code](https://claude.com/claude-code) CLI as one static file,
`FROM scratch`: `/usr/local/bin/claude` is the entrypoint and the only file.
No shell, Node, git or MCP servers.
Multi-arch: `linux/amd64` + `linux/arm64`.

The file is a small static launcher with Claude Code's native (musl) binary
and its loader packed inside. On first run it unpacks them (~285MB) into
`claude-<id>-<uid>` under `$TMPDIR`, `/tmp` or `/dev/shm`, whichever is
writable and allows exec, and reuses that copy on later runs. In this image
the launcher creates `/tmp` itself when run as root; a non-root user or a
read-only root filesystem needs one mounted: `--tmpfs /tmp:exec` (Docker
mounts `--tmpfs` and `/dev/shm` noexec by default), or `TMPDIR` pointing at
a dir that allows exec.

Tags track the upstream [`@anthropic-ai/claude-code`](https://www.npmjs.com/package/@anthropic-ai/claude-code)
version. `:latest` is rebuilt nightly.

## Usage

Copy the file into your own image. Any Linux image of the same arch works,
whatever its libc (or none):

```dockerfile
FROM ubuntu:24.04
COPY --from=ghcr.io/cnuss/claude-code /usr/local/bin/claude /usr/local/bin/claude
```

Or run it as is:

```sh
docker run -it --rm -e ANTHROPIC_API_KEY \
  -v "$PWD":/work -w /work \
  ghcr.io/cnuss/claude-code
```

Arguments go straight to `claude`:

```sh
docker run --rm -e ANTHROPIC_API_KEY ghcr.io/cnuss/claude-code -p "hello"
```

## Build locally

```sh
docker build -t claude-code --build-arg VERSION="$(./1-claude-code/version.sh)" 1-claude-code/
```
