# claude-code

[Claude Code](https://claude.com/claude-code) CLI on Ubuntu 24.04, with Node 22,
git, and ripgrep. Runs as a non-root `claude` user (uid 1000). Multi-arch:
`linux/amd64` + `linux/arm64`.

Tags track the upstream [`@anthropic-ai/claude-code`](https://www.npmjs.com/package/@anthropic-ai/claude-code)
version — `:latest` is rebuilt nightly.

## Usage

Run interactively against the current directory:

```sh
docker run -it --rm \
  -v "$PWD":/workspace \
  -v "$HOME/.claude":/home/claude/.claude \
  ghcr.io/cnuss/claude-code
```

Mounting `~/.claude` persists authentication and settings between runs. To
authenticate with an API key instead:

```sh
docker run -it --rm \
  -e ANTHROPIC_API_KEY \
  -v "$PWD":/workspace \
  ghcr.io/cnuss/claude-code
```

Pin a specific Claude Code version:

```sh
docker run -it --rm -v "$PWD":/workspace ghcr.io/cnuss/claude-code:2.0.14
```

The entrypoint is `claude`, so arguments pass straight through:

```sh
docker run -it --rm -v "$PWD":/workspace ghcr.io/cnuss/claude-code -p "explain this codebase"
```

## Build locally

```sh
docker build -t claude-code --build-arg VERSION="$(./version.sh)" claude-code/
```
