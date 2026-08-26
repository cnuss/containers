FROM ghcr.io/cnuss/ubuntu:24.04

ARG VERSION=latest
ARG DEBIAN_FRONTEND=noninteractive

# npm global prefix in a user-owned dir so claude's auto-updater can write
# (it lives outside the volume mounts on purpose: a volume would shadow the
# image's install)
ENV npm_config_prefix=/opt/claude
ENV PATH="/opt/claude/bin:${PATH}"

# NodeSource apt repo is preconfigured in the base image
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code@${VERSION} chrome-devtools-mcp \
    && chown -R user:user /opt/claude \
    # Volume mountpoint, user-owned so fresh volumes are writable by uid 1000
    && install -d -o user -g user /home/user/.claude

COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint

# Keep all Claude Code state (including the ~/.claude.json config that
# otherwise sits at the home root) inside .claude, where a volume can hold it
ENV CLAUDE_CONFIG_DIR=/home/user/.claude

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Claude Code CLI on Ubuntu 24.04, with Node 22, git, and ripgrep" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-claude-code/README.md" \
    org.opencontainers.image.licenses="MIT"

USER user
WORKDIR /workspace
ENTRYPOINT ["entrypoint"]
