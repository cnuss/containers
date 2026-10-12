# linux

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. The image is `FROM scratch` and holds no OS at
all — just a statically linked kernel at `/uml/linux` and a BusyBox root
filesystem at `/uml/rootfs`, which the guest mounts with hostfs. Boots in about
a second, no `--privileged` needed. `linux/amd64` only: UML has no arm64 port.

The kernel and root filesystem are built in a stage on stock `ubuntu:24.04`;
none of it ships in the final image. It is a tier-0 image — it depends on
nothing else in this repo, so tier-1 images can build `FROM` it. Tags track the kernel release, pinned to the 6.18 LTS (`:6.18`) in
`version.sh`, plus `:latest`, rebuilt nightly.

## Usage

With no arguments, the guest's `/init` prints `uname -a`, `/proc/cpuinfo`
(which reports `User Mode Linux`, not the host CPU), memory, its root mount,
and its own process table, then powers off with exit code 0:

```sh
docker run --rm ghcr.io/cnuss/linux
```

Arguments are a command, joined with spaces and run in the guest with
`/bin/sh -c`; the container exits with its status:

```sh
docker run --rm ghcr.io/cnuss/linux uname -a
docker run --rm ghcr.io/cnuss/linux 'cat /proc/cpuinfo | grep model'
```

An interactive shell in the guest (`exit` to leave; job control and `^C`
work):

```sh
docker run --rm -it ghcr.io/cnuss/linux sh
```

Extra kernel parameters go in `UML_ARGS`, e.g. more guest RAM than the default
128 MB:

```sh
docker run --rm -e UML_ARGS=mem=256M ghcr.io/cnuss/linux free -m
```

The command's stdin is the console when a terminal is attached (`-t`) and
`/dev/null` otherwise; piping input in isn't supported.

`--init` makes `docker stop` immediate. Without it the entrypoint script is
PID 1, which ignores `SIGTERM`, so Docker kills the container after its 10 s
grace period.

## Building on it

`ENTRYPOINT` and `SHELL` are the same wrapper, `/uml/bin/run`, so `RUN` in an
image built `FROM` this one runs inside the guest too — with the guest's
`/bin/sh -c`, as root, from `/` — and the build fails if the command does.
Because the guest's root is a directory in the image, whatever a `RUN` writes
there becomes part of that layer:

```dockerfile
FROM ghcr.io/cnuss/linux
RUN echo 'built inside UML' > /etc/motd && uname -r > /etc/kernel-release
# COPY, WORKDIR, and friends act on the container, not the guest:
# the guest's / is /uml/rootfs
COPY app.sh /uml/rootfs/bin/app.sh
```

`RUN` steps see neither the build's `WORKDIR` nor its `ENV` — except
`UML_ARGS`, which the wrapper reads. The wrapper hands the command over in a
file (`/.uml-run`, removed afterwards) rather than on the kernel command line,
which mangles quotes and panics past 32 words, and the guest reports the exit
status through UML's `/proc/exitcode`.

## How it runs

- **Root filesystem:** `root=/dev/root rootfstype=hostfs rootflags=/uml/rootfs`
  — the guest's `/` is the container's `/uml/rootfs` directory, so guest
  writes are ordinary file writes: in the image layer during a build, in the
  container's layer at runtime (gone with `--rm`, kept by `docker commit`).
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  `TMPDIR=/uml/tmp`, a directory in the image's own filesystem; mounting a
  (noexec) tmpfs over `/tmp` doesn't affect it.
- **Console:** input comes from the container's stdin only when it is a
  terminal. UML hangs up a console whose input reaches EOF — and a build's
  stdin always has — after which every write to it fails.
- **Process tree:** `/uml/bin/run` stays the parent of UML rather than
  `exec`ing it, so UML is never PID 1: a guest kernel panic makes UML
  `abort()`, which a PID 1 would ignore and spin on forever.

## Build locally

```sh
docker build -t ghcr.io/cnuss/linux --build-arg VERSION="$(./0-linux/version.sh)" 0-linux/
docker run --rm ghcr.io/cnuss/linux
```
