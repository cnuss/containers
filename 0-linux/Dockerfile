# User-mode Linux in a container.
#
# Stage 1 builds a statically linked UML kernel and a tiny BusyBox root
# filesystem. Stage 2 is FROM scratch: the final image holds no OS at all, just
# the kernel binary and the guest's root directory, which the guest mounts
# through hostfs. The "OS" inside the container is a Linux kernel running as a
# process, with its own process tree.
#
#   docker run --rm -it ghcr.io/cnuss/linux         # interactive shell in the guest (CMD is sh)
#   docker run --rm ghcr.io/cnuss/linux uname -a    # run a command in the guest
#
# ENTRYPOINT and SHELL are the same wrapper, /uml/bin/run, so images built
# FROM this one run their RUN instructions inside the guest too, and what
# those write to the guest's root lands in the image layer. The container's
# /bin/sh and /usr/bin/env are symlinks to it, so docker exec, exec-form RUN
# and #!/usr/bin/env scripts land in the guest as well.

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

# Root filesystem: BusyBox plus an init script, in a plain directory
COPY init /rootfs/init
RUN mkdir -p /rootfs/bin /rootfs/proc /rootfs/sys /rootfs/dev /rootfs/tmp \
    && chmod 1777 /rootfs/tmp \
    && cp /bin/busybox /rootfs/bin/busybox \
    && for app in $(/rootfs/bin/busybox --list); do [ "$app" = busybox ] || ln -sf busybox "/rootfs/bin/$app"; done \
    && chmod +x /rootfs/init \
    # The container's /bin/sh and /usr/bin/env: run under those names, the
    # RUN wrapper runs the guest's sh or env with the same arguments
    && mkdir -p /shims/bin /shims/usr/bin \
    && ln -s /uml/bin/run /shims/bin/sh \
    && ln -s /uml/bin/run /shims/usr/bin/env

FROM scratch
COPY --from=build /src/linux/linux /uml/linux
# The guest's root filesystem, as a directory: the guest mounts it with
# hostfs, so its writes are ordinary file writes in the container — kept in
# the image layer during a build, in the container's own layer at runtime
COPY --from=build /rootfs /uml/rootfs
# UML backs its "physical RAM" with a file in $TMPDIR that it maps PROT_EXEC,
# so the directory must not be mounted noexec. Docker mounts /dev/shm and any
# --tmpfs noexec by default, so use a directory baked into the image instead,
# out of the way of whatever gets mounted over /tmp at runtime.
COPY --from=build /rootfs/tmp /uml/tmp
ENV TMPDIR=/uml/tmp
# Container-side tools for the RUN wrapper. BusyBox gets its own copy rather
# than borrowing the guest's, which a RUN step is free to replace or delete
COPY --from=build /bin/busybox /uml/bin/busybox
COPY --chmod=755 run /uml/bin/run
COPY --from=build /shims/ /

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="User-mode Linux: a Linux kernel booted as a process, in a FROM scratch image" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/0-linux/README.md" \
    org.opencontainers.image.licenses="MIT"

SHELL ["/uml/bin/run"]
ENTRYPOINT ["/uml/bin/run"]
CMD ["sh"]
