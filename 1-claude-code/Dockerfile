FROM node:24-alpine AS build

ARG VERSION
ARG TARGETARCH

RUN apk add --no-cache gcc musl-dev patchelf

# Named build context: --build-context launcher=launcher
COPY --from=launcher launcher.c pack.mjs /src/

RUN set -eux; \
    case "$TARGETARCH" in amd64) arch=x64 ;; arm64) arch=arm64 ;; *) exit 1 ;; esac; \
    cd /tmp; \
    npm pack --silent "@anthropic-ai/claude-code-linux-${arch}-musl@${VERSION}"; \
    tar xzf anthropic-ai-claude-code-linux-*.tgz; \
    patchelf --set-interpreter "/$(printf '%0240d' 0 | tr 0 x)" package/claude; \
    gcc -static -Os -s -o launcher /src/launcher.c; \
    # The image pins the version, so the self-updater is off by default
    node /src/pack.mjs --launcher launcher --out /claude --exec claude=package/claude \
        --file "ld.so=/lib/ld-musl-$(uname -m).so.1" --interp claude:ld.so \
        --env DISABLE_AUTOUPDATER=1

FROM scratch AS smoke

COPY --from=build /claude /usr/local/bin/claude
RUN ["/usr/local/bin/claude", "--version"]

FROM scratch

# From smoke, not build, so the smoke test can't be skipped
COPY --from=smoke /usr/local/bin/claude /usr/local/bin/claude

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Claude Code CLI as a single static file, FROM scratch" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-claude-code/README.md" \
    org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/claude"]
