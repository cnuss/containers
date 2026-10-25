FROM node:24-alpine AS build

ARG VERSION
ARG TARGETARCH

RUN apk add --no-cache gcc musl-dev

# Named build context: --build-context launcher=launcher
COPY --from=launcher launcher.c pack.mjs /src/

# The platform package's bin/codex is a static musl binary; the launcher only
# adds a CA bundle, used when the host has no CA store, and turns off the
# startup update check (codex has no env var for it): the image pins the
# version
RUN set -eux; \
    case "$TARGETARCH" in amd64) arch=x64 triple=x86_64 ;; arm64) arch=arm64 triple=aarch64 ;; *) exit 1 ;; esac; \
    cd /tmp; \
    npm pack --silent "@openai/codex@${VERSION}-linux-${arch}"; \
    tar xzf openai-codex-*.tgz; \
    gcc -static -Os -s -o launcher /src/launcher.c; \
    node /src/pack.mjs --launcher launcher --out /codex \
        --exec "codex=package/vendor/${triple}-unknown-linux-musl/bin/codex" \
        --file ca.pem=/etc/ssl/certs/ca-certificates.crt --cafile ca.pem \
        --arg=-c --arg=check_for_update_on_startup=false

FROM scratch AS smoke

COPY --from=build /codex /usr/local/bin/codex
RUN ["/usr/local/bin/codex", "--version"]

FROM scratch

# From smoke, not build, so the smoke test can't be skipped
COPY --from=smoke /usr/local/bin/codex /usr/local/bin/codex

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="OpenAI Codex CLI as a single static file, FROM scratch" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-codex/README.md" \
    org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/codex"]
