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

# Kernel release to build, without the leading "v" (version.sh resolves it)
ARG VERSION=6.18
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    build-essential flex bison bc busybox-static curl xz-utils ca-certificates patchelf \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN curl -fsSL "https://cdn.kernel.org/pub/linux/kernel/v${VERSION%%.*}.x/linux-${VERSION}.tar.xz" \
    | tar -xJ \
    && mv "linux-${VERSION}" linux

WORKDIR /src/linux

# Build the kernel as a host executable (ARCH=um).
RUN make ARCH=um defconfig \
    && ./scripts/config --enable HOSTFS --enable DEVTMPFS --enable DEVTMPFS_MOUNT --disable BLK_DEV_UBD \
    && ./scripts/config --enable UML_NET_VECTOR \
    && make ARCH=um olddefconfig \
    && make ARCH=um -j"$(nproc)" linux \
    && strip linux

# The vector network driver rules out STATIC_LINK (olddefconfig drops it), so
# UML is dynamically linked: its glibc goes in /uml/lib, where the binary
# looks for it, leaving the rest of the filesystem free of a libc. libgcc_s
# is dlopen()ed by glibc for pthread_cancel, and only DT_RPATH (not RUNPATH)
# covers that dlopen
RUN interp="$(patchelf --print-interpreter linux)" \
    && mkdir -p /out/uml/lib \
    && cp -L "$interp" "$(gcc -print-file-name=libgcc_s.so.1)" \
    $(ldd linux | awk '$2 == "=>" && $3 ~ /^\// { print $3 }') /out/uml/lib/ \
    && ln -s "${interp##*/}" /out/uml/lib/ld.so \
    && patchelf --set-interpreter "/uml/lib/${interp##*/}" --force-rpath --set-rpath /uml/lib linux

# gvproxy (gvisor-tap-vsock): a user-space network stack for the guest's vec0,
# which reaches it over a unix seqpacket socket (UML's "bess" transport). The
# guest's connections leave from the container's own network namespace, with
# no extra privileges. It's dynamically linked too, so /uml/bin/run starts it
# through /uml/lib/ld.so (patchelf breaks Go binaries). /uml/etc is where the
# guest mounts its resolv.conf
ARG GVPROXY_VERSION=0.9.0
RUN mkdir -p /out/uml/bin /out/uml/etc \
    && file="gvproxy-linux-$(dpkg --print-architecture)" \
    && url="https://github.com/containers/gvisor-tap-vsock/releases/download/v${GVPROXY_VERSION}" \
    && curl -fsSLo "/out/uml/bin/gvproxy" "$url/$file" \
    && curl -fsSL "$url/sha256sums" | grep " $file\$" | sed "s| .*|  /out/uml/bin/gvproxy|" | sha256sum -c - \
    && chmod 755 /out/uml/bin/gvproxy

# The image's filesystem, which is the guest's too: the kernel, init, runas, the
# wrapper and BusyBox under /uml, and BusyBox's commands in /bin and
# /usr/bin/env as symlinks to the wrapper. In the container they lead into a
# guest; inside it, init bind-mounts BusyBox over the wrapper, so there they
# are BusyBox applets. /dev, /proc, /sys and /etc exist up front so builds and
# containers don't add them as mount points. A root-only /etc/passwd and
# /etc/group, as in alpine: BuildKit needs them to resolve USER root or a
# bare USER <uid>.
COPY init run runas.c /out/uml/bin/
RUN mkdir -p /out/bin /out/usr/bin /out/uml/tmp /out/tmp \
    /out/dev /out/proc /out/sys /out/etc /out/root \
    && chmod 1777 /out/tmp /out/uml/tmp \
    && chmod 700 /out/root \
    && echo 'root:x:0:0:root:/root:/bin/sh' > /out/etc/passwd \
    && echo 'root:x:0:' > /out/etc/group \
    && gcc -static -Os -s -o /out/uml/bin/runas /out/uml/bin/runas.c \
    && rm /out/uml/bin/runas.c \
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
