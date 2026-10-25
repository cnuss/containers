# codex

The [OpenAI Codex](https://github.com/openai/codex) CLI as one static file,
`FROM scratch`: `/usr/local/bin/codex` is the entrypoint and the only file.
No shell, Node, ripgrep or bubblewrap. Multi-arch: `linux/amd64` +
`linux/arm64`.

The file is the shared [launcher](../launcher) with codex's static binary
and a CA bundle packed inside. On first run it unpacks them (~270MB) into
`codex-<id>-<uid>` under `$TMPDIR`, `/tmp` or `/dev/shm`, whichever is
writable and allows exec, and reuses that copy on later runs. codex verifies
TLS against the system CA store; when the host has none (and
`SSL_CERT_FILE`/`SSL_CERT_DIR` are unset), the launcher points
`SSL_CERT_FILE` at the packed bundle. In this image the launcher creates
`/tmp` itself when run as root; a non-root user or a read-only root
filesystem needs one mounted: `--tmpfs /tmp:exec` (Docker mounts `--tmpfs`
and `/dev/shm` noexec by default), or `TMPDIR` pointing at a dir that allows
exec.

Tags track the upstream [`@openai/codex`](https://www.npmjs.com/package/@openai/codex)
version. `:latest` is rebuilt nightly.

## Usage

Copy the binary into your own image. Any Linux image of the same arch works,
whatever its libc (or none):

```dockerfile
FROM ubuntu:24.04
COPY --from=ghcr.io/cnuss/codex /usr/local/bin/codex /usr/local/bin/codex
```

Or run it as is. codex keeps its state in `~/.codex` (or `$CODEX_HOME`), so a
non-root user needs a writable `HOME`:

```sh
docker run -it --rm -e CODEX_API_KEY \
  -v "$PWD":/work -w /work \
  ghcr.io/cnuss/codex
```

```sh
docker run --rm -e CODEX_API_KEY ghcr.io/cnuss/codex exec --skip-git-repo-check "hello"
```

## Build locally

From the repo root (the [launcher](../launcher) is a named build context):

```sh
docker build -t codex --build-context launcher=launcher \
  --build-arg VERSION="$(./1-codex/version.sh)" 1-codex/
```
