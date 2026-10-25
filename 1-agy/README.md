# agy

The [Google Antigravity](https://antigravity.google) CLI (`agy`) as one
static file, `FROM scratch`: `/usr/local/bin/agy` is the entrypoint and the
only file. No shell or anything else. Multi-arch: `linux/amd64` +
`linux/arm64`.

The file is the shared [launcher](../launcher) with agy's static (musl)
binary and a CA bundle packed inside. On first run it unpacks them (~195MB)
into `agy-<id>-<uid>` under `$TMPDIR`, `/tmp` or `/dev/shm`, whichever is
writable and allows exec, and reuses that copy on later runs. agy verifies
TLS against the system CA store; when the host has none (and
`SSL_CERT_FILE`/`SSL_CERT_DIR` are unset), the launcher points
`SSL_CERT_FILE` at the packed bundle. In this image the launcher creates
`/tmp` itself when run as root; a non-root user or a read-only root
filesystem needs one mounted: `--tmpfs /tmp:exec` (Docker mounts `--tmpfs`
and `/dev/shm` noexec by default), or `TMPDIR` pointing at a dir that allows
exec.

Tags track upstream's release version. `:latest` is rebuilt nightly.

## Usage

Copy the file into your own image. Any Linux image of the same arch works,
whatever its libc (or none):

```dockerfile
FROM ubuntu:24.04
COPY --from=ghcr.io/cnuss/agy /usr/local/bin/agy /usr/local/bin/agy
```

Or run it as is. agy keeps its state in `~/.gemini/antigravity-cli`, so a
non-root user needs a writable `HOME`. Without a browser, it signs in with a
Google account by printing a URL and reading the code back. To use a Gemini
API key instead, set `"modelProvider": "gemini"` in
`~/.gemini/antigravity-cli/settings.json` and pass `GEMINI_API_KEY`:

```sh
docker run -it --rm -e GEMINI_API_KEY \
  -e HOME=/home -v "$HOME/.gemini":/home/.gemini \
  -v "$PWD":/work -w /work \
  ghcr.io/cnuss/agy
```

```sh
docker run --rm -e GEMINI_API_KEY -e HOME=/home -v "$HOME/.gemini":/home/.gemini \
  ghcr.io/cnuss/agy -p "hello"
```

The image pins agy's version, so the launcher turns agy's background
self-updater off by setting `AGY_CLI_DISABLE_AUTO_UPDATE=true` (agy accepts
only `true`) unless you set it yourself.

## Build locally

From the repo root (the [launcher](../launcher) is a named build context):

```sh
docker build -t agy --build-context launcher=launcher \
  --build-arg VERSION="$(./1-agy/version.sh)" 1-agy/
```
