FROM ghcr.io/cnuss/ubuntu:24.04

ARG VERSION=latest
ARG DEBIAN_FRONTEND=noninteractive

# NodeSource apt repo is preconfigured in the base image
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code@${VERSION}

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Claude Code CLI on Ubuntu 24.04, with Node 22, git, and ripgrep" \
    org.opencontainers.image.licenses="MIT"

USER user
WORKDIR /workspace
ENTRYPOINT ["claude"]
