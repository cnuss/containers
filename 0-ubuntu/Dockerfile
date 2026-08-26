ARG VERSION=24.04

FROM ubuntu:${VERSION} AS combined

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    gnupg \
    less \
    ripgrep \
    zsh \
    && install -d -m 0755 /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
    > /etc/apt/sources.list.d/nodesource.list \
    && curl -fsSL https://xpra.org/xpra.asc \
    | gpg --dearmor -o /etc/apt/keyrings/xpra.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/xpra.gpg] https://xpra.org/ noble main" \
    > /etc/apt/sources.list.d/xpra.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
    xpra \
    xpra-x11 \
    xpra-html5 \
    xvfb \
    && rm -rf /var/lib/apt/lists/*

# Replace the stock ubuntu user so uid 1000 maps cleanly onto host volume mounts
RUN userdel -r ubuntu \
    && useradd -m -u 1000 -s /bin/zsh user \
    && install -d -o user -g user /home/user \
    && install -d -m 0700 -o user -g user /run/user/1000 \
    && install -d -o user -g user /run/xpra \
    # XDG base directories, user-owned: a named volume adopts the image
    # dir's ownership on first use, but only if the dir exists — bake them
    # so fresh volumes are writable by uid 1000
    && install -d -o user -g user \
    /home/user/.config \
    /home/user/.cache \
    /home/user/.local

# Flatten to a single layer shared by every downstream image
FROM scratch
COPY --from=combined / /

ENV XDG_RUNTIME_DIR=/run/user/1000
ENV XDG_CACHE_HOME=/home/user/.cache

USER user
WORKDIR /home/user

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Ubuntu 24.04 base with common tools" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/0-ubuntu/README.md" \
    org.opencontainers.image.licenses="MIT"

CMD ["/bin/zsh"]
