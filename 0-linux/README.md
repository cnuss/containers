# linux

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. It works like `alpine`, except that every
command runs on its own kernel: `docker run`, `docker exec`, `RUN` and shell
scripts all land in a guest whose root filesystem is the container's own, so
`WORKDIR`, `ENV`, `COPY` and volumes mean the same thing there. Boots in about
a second, no `--privileged` needed.

The image is `FROM scratch`: a statically linked kernel and its plumbing under
`/uml`, and BusyBox's commands in `/bin`. `linux/amd64` only: mainline UML has
no arm64 port, and the image can't run emulated on arm64 hosts either (UML
controls its guest processes with `ptrace`, which QEMU user mode doesn't
implement).

The kernel is built in a stage on stock `ubuntu:24.04`. It is a tier-0 image —
it depends on nothing else in this repo, so tier-1 images can build `FROM` it.
Tags track the kernel release, pinned to the 6.18 LTS (`:6.18`) in
`version.sh`, plus `:latest`, rebuilt nightly.

## Usage

As with `alpine`, there is no `ENTRYPOINT` and `CMD` is `/bin/sh`, so with no
arguments you get a shell — here, in the guest (`exit` to leave; job control
and `^C` work):

```sh
docker run --rm -it ghcr.io/cnuss/linux
```

Arguments are a command, run in the guest; the container exits with its
status. `uname` reports the guest's kernel, and `/proc/cpuinfo` says
`User Mode Linux`, not the host CPU:

```sh
docker run --rm ghcr.io/cnuss/linux uname -a
docker run --rm ghcr.io/cnuss/linux sh -c 'grep -E "vendor_id|model name" /proc/cpuinfo'
docker run --rm -v "$PWD":/work -w /work ghcr.io/cnuss/linux ls
```

The command runs as root with the container's working directory, environment
and hostname. Extra kernel parameters go in `UML_ARGS`, e.g. more guest RAM
than the default 128 MB:

```sh
docker run --rm -e UML_ARGS=mem=256M ghcr.io/cnuss/linux free -m
```

The command's stdin is the console when a terminal is attached (`-t`) and
`/dev/null` otherwise; piping input in isn't supported.

`--init` makes `docker stop` immediate. Without it the command's wrapper is
PID 1, which ignores `SIGTERM`, so Docker kills the container after its 10 s
grace period.

## Building on it

`SHELL` is the default `/bin/sh -c`, spelled out — and `/bin/sh` leads into the
guest — so `RUN` in an image built `FROM` this one runs inside the guest, with
the step's `WORKDIR` and `ENV`, and the build fails if the command does.
Exec-form `RUN ["uname", "-r"]` lands there too. Whatever a step writes
becomes part of its layer:

```dockerfile
FROM ghcr.io/cnuss/linux
WORKDIR /app
COPY . .
ENV MODE=release
RUN ./build.sh "$MODE"     # runs in the guest, in /app, with MODE set
```

## Every command is a door into the guest

Each of BusyBox's commands in `/bin` (`sh`, `ls`, `uname`, ... and `busybox`
itself), and `/usr/bin/env`, is a symlink to `/uml/bin/run`. Run under one of
those names, the wrapper boots a guest and runs that command there with
exactly the same arguments. So `docker run IMAGE uname -a`, `docker exec
CONTAINER ls`, health checks and `#!/bin/sh` or `#!/usr/bin/env` scripts all
land in a guest. Inside the guest, init bind-mounts BusyBox over
`/uml/bin/run`, a mount the container never sees, so there the same symlinks
are plain BusyBox applets.

Every call boots its own guest: `docker exec` doesn't join the guest the
container started with but boots another kernel on the same filesystem, so
files are shared and processes aren't. To run something in the container
itself, on the host kernel, use BusyBox directly:
`docker exec CONTAINER /uml/bin/busybox ls`.

## How it runs

- **Root filesystem:** `root=/dev/root rootfstype=hostfs rootflags=/` — the
  guest's `/` is the container's `/`, with the guest's own `/dev`, `/proc` and
  `/sys` mounted over it. Guest writes are ordinary file writes: in the image
  layer during a build, in the container's layer at runtime (gone with `--rm`,
  kept by `docker commit`), or in a volume.
- **Handoff:** each call writes a small script to `/uml/tmp/run.<pid>` —
  export the caller's environment, `cd` to its working directory, `exec` the
  command, every word single-quoted — that the guest's init runs. Not the
  kernel command line, which mangles quotes and panics past 32 words. The
  guest reports the exit status through UML's `/proc/exitcode`.
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  the wrapper sets `TMPDIR=/uml/tmp` for UML alone; mounting a (noexec) tmpfs
  over `/tmp` doesn't affect it.
- **Console:** input comes from the container's stdin only when it is a
  terminal. UML hangs up a console whose input reaches EOF — and a build's
  stdin always has — after which every write to it fails.
- **Process tree:** the wrapper stays the parent of UML rather than
  `exec`ing it, so UML is never PID 1: a guest kernel panic makes UML
  `abort()`, which a PID 1 would ignore and spin on forever.

## Build locally

```sh
docker build -t ghcr.io/cnuss/linux --build-arg VERSION="$(./0-linux/version.sh)" 0-linux/
docker run --rm ghcr.io/cnuss/linux uname -a
```
