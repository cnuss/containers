FROM node:24-alpine AS build

ARG VERSION
ARG TARGETARCH

RUN apk add --no-cache gcc musl-dev patchelf

COPY launcher.c pack.mjs /src/

RUN set -eux; \
    case "$TARGETARCH" in amd64) arch=x64 ;; arm64) arch=arm64 ;; *) exit 1 ;; esac; \
    cd /tmp; \
    npm pack --silent "@anthropic-ai/claude-code-linux-${arch}-musl@${VERSION}"; \
    tar xzf anthropic-ai-claude-code-linux-*.tgz; \
    patchelf --set-interpreter "/$(printf '%0240d' 0 | tr 0 x)" package/claude; \
    gcc -static -Os -s -o launcher /src/launcher.c; \
    install -d /out/usr/local/bin; \
    node /src/pack.mjs launcher "/lib/ld-musl-$(uname -m).so.1" package/claude /out/usr/local/bin/claude; \
    /out/usr/local/bin/claude --version; \
    rm -rf /tmp/claude-*; \
    # claude mkdirs under /tmp at startup; non-root users can't create it
    install -d -m 1777 /out/tmp

FROM scratch

COPY --from=build /out/ /

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Claude Code CLI as a single static binary, FROM scratch" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-claude-code/README.md" \
    org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/claude"]
