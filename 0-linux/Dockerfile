# User-mode Linux in a container.
#
# Stage 1 builds a statically linked UML kernel. Stage 2 is FROM scratch: the
# kernel and its plumbing under /uml, and BusyBox's commands in /bin — each a
# symlink to the wrapper /uml/bin/run, which runs that command in a guest
# that mounts the container's whole filesystem as its root through hostfs.
# The "OS" inside the container is a Linux kernel running as a process, with
# its own process tree.
#
#   docker run --rm -it ghcr.io/cnuss/linux         # shell in the guest (CMD is /bin/sh)
#   docker run --rm ghcr.io/cnuss/linux uname -a    # a command in the guest
#
# No ENTRYPOINT, as in alpine; SHELL is the default /bin/sh -c, spelled out,
# so images built FROM this one run their RUN instructions inside the guest
# too, with the step's WORKDIR and ENV, and what those write lands in the
# image layer.

# Stock ubuntu rather than ghcr.io/cnuss/ubuntu: this is a tier-0 image, built
# alongside that base rather than after it, and nothing from this stage ships
FROM ubuntu:24.04 AS build

# Kernel release to build, without the leading "v" (version.sh pins it)
ARG VERSION=6.18
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential flex bison bc busybox-static git ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --depth 1 --branch "v${VERSION}" https://github.com/torvalds/linux.git

# Build the kernel as a host executable (ARCH=um). Static linking means the
# final image needs no libc.
WORKDIR /src/linux
RUN make ARCH=um defconfig \
    && ./scripts/config --enable STATIC_LINK --enable HOSTFS --enable DEVTMPFS --enable DEVTMPFS_MOUNT --disable BLK_DEV_UBD \
    && make ARCH=um olddefconfig \
    && make ARCH=um -j"$(nproc)" linux \
    && strip linux

# The image's filesystem, which is the guest's too: the kernel, init, the
# wrapper and BusyBox under /uml, and BusyBox's commands in /bin and
# /usr/bin/env as symlinks to the wrapper. In the container they lead into a
# guest; inside it, init bind-mounts BusyBox over the wrapper, so there they
# are BusyBox applets. /dev, /proc, /sys and /etc exist up front so builds and
# containers don't add them as mount points.
COPY init run /out/uml/bin/
RUN mkdir -p /out/bin /out/usr/bin /out/uml/tmp /out/tmp \
    /out/dev /out/proc /out/sys /out/etc \
    && chmod 1777 /out/tmp \
    && chmod 755 /out/uml/bin/init /out/uml/bin/run \
    && cp /src/linux/linux /out/uml/linux \
    && cp /bin/busybox /out/uml/bin/busybox \
    && for app in busybox $(/bin/busybox --list); do ln -sf /uml/bin/run "/out/bin/$app"; done \
    && ln -s /uml/bin/run /out/usr/bin/env

FROM scratch
COPY --from=build /out/ /

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="User-mode Linux: a Linux kernel booted as a process, in a FROM scratch image" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/0-linux/README.md" \
    org.opencontainers.image.licenses="MIT"

SHELL ["/bin/sh", "-c"]
CMD ["/bin/sh"]
