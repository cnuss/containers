# tsfifo: a Tailscale node (tsnet) whose TUN device is a SOCK_SEQPACKET unix
# socket, for hosts with no /dev/net/tun or CAP_NET_ADMIN.
#
# FROM scratch with one static binary, meant to be copied into other images:
#
#   COPY --from=ghcr.io/cnuss/tsfifo /tsfifo /usr/local/bin/tsfifo

FROM golang:1.27 AS build

# Tailscale release to build against, without the leading "v" (version.sh
# resolves it)
ARG VERSION=1.104.1

WORKDIR /src
COPY go.mod go.sum ./
RUN go get "tailscale.com@v${VERSION}" && go mod download
COPY *.go ./
RUN go mod tidy \
    && CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X tailscale.com/version.longStamp=${VERSION}-tsfifo -X tailscale.com/version.shortStamp=${VERSION}" \
    -o /out/tsfifo . \
    && mkdir -p -m 1777 /out/tmp \
    && mkdir -p /out/etc/ssl/certs \
    && cp /etc/ssl/certs/ca-certificates.crt /out/etc/ssl/certs/

FROM scratch
COPY --from=build /out/ /
ENTRYPOINT ["/tsfifo"]
