# LKL, the Linux Kernel Library: a Linux kernel built as a shared library, and
# its LD_PRELOAD "hijack" library, which moves a program's sockets onto that
# kernel's network stack inside the program's own process. No privileges,
# ptrace or seccomp needed, so it runs where those are denied (AWS Lambda).
#
# FROM scratch with the two libraries and LKL's headers under /opt/lkl, meant
# to be copied into other images (glibc 2.39 or newer):
#
#   COPY --from=ghcr.io/cnuss/lkl /opt/lkl /opt/lkl
#   LKL_HIJACK_CONFIG_FILE=lkl.json LD_PRELOAD=/opt/lkl/liblkl-hijack.so PROGRAM
#   gcc -I/opt/lkl/include -o prog prog.c -L/opt/lkl -llkl -Wl,-rpath,/opt/lkl

# Stock ubuntu rather than ghcr.io/cnuss/ubuntu: this is a tier-0 image, built
# alongside that base rather than after it, and nothing from this stage ships
FROM ubuntu:24.04 AS build

# "<kernel version>-<lkl/linux commit>" (version.sh resolves it)
ARG VERSION=6.12.0-d0f76a77e8c2
ARG DEBIAN_FRONTEND=noninteractive

# libjsmn-dev: without jsmn.h the hijack's JSON config parser is compiled out,
# and every config file is silently ignored
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential flex bison bc python3 curl ca-certificates libelf-dev libjsmn-dev patch \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN curl -fsSL "https://codeload.github.com/lkl/linux/tar.gz/${VERSION##*-}" | tar -xz \
    && mv linux-* linux

WORKDIR /src/linux

# The "fifo" interface type: Ethernet frames over a pair of FIFOs, each frame
# after its length as a 4-byte big-endian integer, MTU 1280 by default
COPY fifo.patch /tmp/
RUN patch -p1 < /tmp/fifo.patch

RUN make ARCH=lkl defconfig \
    && ./scripts/config \
    --enable WIREGUARD --enable TUN --enable VETH --enable VXLAN --enable NET_IPGRE \
    --enable IP_ADVANCED_ROUTER --enable IP_MULTIPLE_TABLES --enable IPV6_MULTIPLE_TABLES \
    --enable NETFILTER --enable NF_CONNTRACK --enable NF_NAT --enable NF_TABLES \
    --enable NF_TABLES_INET --enable NFT_CT --enable NFT_NAT --enable NFT_MASQ \
    && make ARCH=lkl olddefconfig \
    && make -C tools/lkl -j"$(nproc)"

# The hijack is built on liblkl.so rather than taken from tools/lkl, whose
# liblkl-hijack.so links the kernel in statically and hangs at boot: the
# kernel's clock never starts. The checks boot it in a process and fail the
# build if it doesn't come up (a booted kernel runs in threads of its own),
# and build a program against the shipped headers
RUN mkdir -p /out/opt/lkl \
    && cd tools/lkl/lib/hijack \
    && gcc -shared -fPIC -O2 -std=gnu11 -D_GNU_SOURCE -I../../include -I.. -I. \
    -o /out/opt/lkl/liblkl-hijack.so hijack.c init.c xlate.c dbg_handler.c preload.c \
    -L.. -llkl -Wl,-rpath,'$ORIGIN' -ldl -lpthread \
    && cp ../liblkl.so /out/opt/lkl/ \
    && cp -r ../../include /out/opt/lkl/include \
    && strip /out/opt/lkl/*.so \
    && echo '{"singlecpu":"0"}' > /tmp/lkl.json \
    && threads=$(timeout 60 env LKL_HIJACK_CONFIG_FILE=/tmp/lkl.json \
    LD_PRELOAD=/out/opt/lkl/liblkl-hijack.so awk '/^Threads:/ { print $2 }' /proc/self/status) \
    && echo "threads with LKL booted: ${threads}" \
    && [ "${threads:-0}" -gt 1 ] \
    && printf '#include <lkl.h>\n#include <lkl_host.h>\nint main(void) { return lkl_init(&lkl_host_ops) || !lkl_netdev_fifo_create("a|b") ? 0 : LKL_NETDEV_FIFO_DEFAULT_MTU; }\n' > /tmp/api.c \
    && gcc -I/out/opt/lkl/include -o /tmp/api /tmp/api.c -L/out/opt/lkl -llkl

FROM scratch
COPY --from=build /out/ /

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="LKL: a Linux kernel as a library, with an LD_PRELOAD library that moves a program's sockets onto it" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/0-lkl/README.md" \
    org.opencontainers.image.licenses="GPL-2.0-only"
