FROM ghcr.io/cnuss/ubuntu:24.04

ARG VERSION=latest
ARG DEBIAN_FRONTEND=noninteractive

# NodeSource apt repo is preconfigured in the base image
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g @anthropic-ai/claude-code@${VERSION}

USER user
WORKDIR /workspace
ENTRYPOINT ["claude"]
