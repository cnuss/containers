# User-mode Linux in a container.
#
# Stage 1 builds a statically linked UML kernel and a tiny BusyBox root
# filesystem. Stage 2 is FROM scratch: the final image holds no OS at all, just
# the kernel binary and the guest's root directory, which the guest mounts
# through hostfs. The "OS" inside the container is a Linux kernel running as
# PID 1's process tree.
#
#   docker run --rm --init ghcr.io/cnuss/uml              # boot, print some facts, power off
#   docker run --rm -it --init ghcr.io/cnuss/uml shell=1  # boot to an interactive shell
#
# Anything after the image name is appended to the UML kernel command line.
# --init matters only when things go wrong: on a guest kernel panic UML calls
# abort(), and a process that is PID 1 ignores SIGABRT, so without --init a
# panicked container spins forever instead of exiting.
#
# SHELL is set, so images built FROM this one run their RUN instructions
# inside the guest, and what those write to the guest's root lands in the
# image layer.

FROM ghcr.io/cnuss/ubuntu:24.04 AS build

# The base image defaults to the unprivileged user; installs need root
USER root

# Kernel release to build, without the leading "v" (version.sh pins it)
ARG VERSION=6.18
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential flex bison bc busybox-static \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone --depth 1 --branch "v${VERSION}" https://github.com/torvalds/linux.git

# Build the kernel as a host executable (ARCH=um). Static linking means the
# final image needs no libc.
WORKDIR /src/linux
RUN make ARCH=um defconfig \
    && ./scripts/config --enable STATIC_LINK --enable HOSTFS --enable DEVTMPFS --disable BLK_DEV_UBD \
    && make ARCH=um olddefconfig \
    && make ARCH=um -j"$(nproc)" linux \
    && strip linux

# Root filesystem: BusyBox plus an init script, in a plain directory
COPY init /rootfs/init
RUN mkdir -p /rootfs/bin /rootfs/proc /rootfs/sys /rootfs/dev /rootfs/tmp \
    && chmod 1777 /rootfs/tmp \
    && cp /bin/busybox /rootfs/bin/busybox \
    && for app in $(/rootfs/bin/busybox --list); do [ "$app" = busybox ] || ln -sf busybox "/rootfs/bin/$app"; done \
    && chmod +x /rootfs/init

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

LABEL org.opencontainers.image.source="https://github.com/cnuss/containers" \
    org.opencontainers.image.description="User-mode Linux: a Linux kernel booted as a process, in a FROM scratch image" \
    org.opencontainers.image.documentation="https://github.com/cnuss/containers/blob/main/1-uml/README.md" \
    org.opencontainers.image.licenses="MIT"

# uml_dir: UML keeps a per-instance directory (pid file, mconsole socket)
# under $HOME, which is / here; inside /uml/tmp it can't leak into a layer
SHELL ["/uml/bin/run"]
ENTRYPOINT ["/uml/linux", "root=/dev/root", "rootfstype=hostfs", "rootflags=/uml/rootfs", "rw", "mem=128M", "init=/init", "con=null", "con0=fd:0,fd:1", "ssl=null", "uml_dir=/uml/tmp", "quiet"]
