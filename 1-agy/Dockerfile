FROM node:24-alpine AS build

ARG VERSION
ARG TARGETARCH

RUN apk add --no-cache gcc musl-dev

# Named build context: --build-context launcher=launcher
COPY --from=launcher launcher.c pack.mjs /src/

# The release server only has a manifest for the latest build, so it must
# still be the version discover resolved. The musl build is static; the
# launcher only adds a CA bundle, used when the host has no CA store, and
# turns off the self-updater: the image pins the version.
RUN set -eux; \
    case "$TARGETARCH" in amd64|arm64) ;; *) exit 1 ;; esac; \
    cd /tmp; \
    wget -qO manifest.json "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/linux_${TARGETARCH}_musl.json"; \
    version=$(node -p 'require("/tmp/manifest.json").version'); \
    if [ "$version" != "$VERSION" ]; then echo "manifest has $version, want $VERSION" >&2; exit 1; fi; \
    wget -qO agy.tar.gz "$(node -p 'require("/tmp/manifest.json").url')"; \
    echo "$(node -p 'require("/tmp/manifest.json").sha512')  agy.tar.gz" | sha512sum -c -; \
    tar xzf agy.tar.gz antigravity; \
    gcc -static -Os -s -o launcher /src/launcher.c; \
    node /src/pack.mjs --launcher launcher --out /agy --exec agy=antigravity \
        --file ca.pem=/etc/ssl/certs/ca-certificates.crt --cafile ca.pem \
        --env AGY_CLI_DISABLE_AUTO_UPDATE=true

FROM scratch AS smoke

COPY --from=build /agy /usr/local/bin/agy
RUN ["/usr/local/bin/agy", "--version"]

FROM scratch

# From smoke, not build, so the smoke test can't be skipped
COPY --from=smoke /usr/local/bin/agy /usr/local/bin/agy

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="Google Antigravity CLI (agy) as a single static file, FROM scratch" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-agy/README.md" \
    org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/agy"]
