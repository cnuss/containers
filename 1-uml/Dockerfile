# User-mode Linux in a container.
#
# Stage 1 builds a statically linked UML kernel and a tiny BusyBox root disk.
# Stage 2 is FROM scratch: the final image holds no OS at all, just the kernel
# binary and its disk image. The "OS" inside the container is a Linux kernel
# running as PID 1's process tree.
#
#   docker run --rm --init ghcr.io/cnuss/uml              # boot, print some facts, power off
#   docker run --rm -it --init ghcr.io/cnuss/uml shell=1  # boot to an interactive shell
#
# Anything after the image name is appended to the UML kernel command line.
# --init matters only when things go wrong: on a guest kernel panic UML calls
# abort(), and a process that is PID 1 ignores SIGABRT, so without --init a
# panicked container spins forever instead of exiting.

FROM ghcr.io/cnuss/ubuntu:24.04 AS build

# The base image defaults to the unprivileged user; installs need root
USER root

# Kernel release to build, without the leading "v" (version.sh pins it)
ARG VERSION=6.18
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential flex bison bc busybox-static e2fsprogs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --depth 1 --branch "v${VERSION}" https://github.com/torvalds/linux.git

# Build the kernel as a host executable (ARCH=um). Static linking means the
# final image needs no libc.
WORKDIR /src/linux
RUN make ARCH=um defconfig \
    && ./scripts/config --enable STATIC_LINK --enable BLK_DEV_UBD --enable DEVTMPFS \
    && make ARCH=um olddefconfig \
    && make ARCH=um -j"$(nproc)" linux \
    && strip linux

# Root filesystem: BusyBox plus an init script, packed into an ext4 image
# that UML will see as the block device /dev/ubda.
COPY init /rootfs/init
RUN mkdir -p /rootfs/bin /rootfs/proc /rootfs/sys /rootfs/dev /rootfs/tmp \
    && cp /bin/busybox /rootfs/bin/busybox \
    && for app in $(/rootfs/bin/busybox --list); do [ "$app" = busybox ] || ln -sf busybox "/rootfs/bin/$app"; done \
    && chmod +x /rootfs/init \
    && mkfs.ext4 -q -E lazy_itable_init=0,lazy_journal_init=0 -L uml-root -d /rootfs /rootfs.img 32M

FROM scratch
# Kept out of / on purpose: UML's COW code resolves the backing file's
# directory, and a file sitting directly in / trips it up (EINVAL).
COPY --from=build /src/linux/linux /uml/linux
COPY --from=build /rootfs.img /uml/rootfs.img
# UML backs its "physical RAM" with a file in $TMPDIR that it maps PROT_EXEC,
# so the directory must not be mounted noexec. Docker mounts /dev/shm and any
# --tmpfs noexec by default, so use a directory baked into the image instead,
# out of the way of whatever gets mounted over /tmp at runtime.
COPY --from=build /rootfs/tmp /uml/tmp
ENV TMPDIR=/uml/tmp

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="User-mode Linux: a Linux kernel booted as a process, in a FROM scratch image" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-uml/README.md" \
    org.opencontainers.image.licenses="MIT"

# ubd0=<cow>,<backing>: UML opens rootfs.img read-only and writes changes to a
# copy-on-write overlay. Opening the image read-write directly would make
# overlayfs copy it up, and UML's timer interrupts that copy-up forever.
ENTRYPOINT ["/uml/linux", "ubd0=/dev/shm/rootfs.cow,/uml/rootfs.img", "root=/dev/ubda", "rw", "mem=128M", "init=/init", "con=null", "con0=fd:0,fd:1", "ssl=null", "quiet"]
