FROM ghcr.io/cnuss/ubuntu:24.04

# The base image defaults to the unprivileged user; installs need root
USER root

# Oh My Zsh has no upstream releases, so there is no version.sh and the tag is
# the build date; VERSION is declared only to consume the build arg CI passes
ARG VERSION=latest
ARG DEBIAN_FRONTEND=noninteractive

# Oh My Zsh lives outside the home directory on purpose: a volume mounted at
# /home/user would shadow an install kept there. It stays user-owned so
# `omz update` can write to it
ENV ZSH=/opt/oh-my-zsh
ENV SHELL=/bin/zsh

# zsh and git come from the base image
RUN git clone --depth 1 https://github.com/ohmyzsh/ohmyzsh.git "$ZSH" \
    && chown -R user:user "$ZSH"

# Baked into the home directory so a fresh volume mounted there is seeded with
# it, and so the user's own edits persist in that volume
COPY --chown=user:user --chmod=644 zshrc /home/user/.zshrc

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Zsh with Oh My Zsh on Ubuntu 24.04" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-zsh/README.md" \
    org.opencontainers.image.licenses="MIT"

USER user
WORKDIR /home/user
ENTRYPOINT ["zsh"]
