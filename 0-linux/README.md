# linux

[User-mode Linux](https://docs.kernel.org/virt/uml/user_mode_linux_howto_v2.html):
a Linux kernel built as an ordinary executable (`ARCH=um`) that boots as a
process inside the container. The guest's root filesystem is the container's
own, so it behaves like an ordinary container image whose commands run on
their own kernel: `WORKDIR`, `ENV`, `COPY` and volumes all mean the same thing
inside the guest. Boots in about a second, no `--privileged` needed.

The image is `FROM scratch`: a statically linked kernel and its plumbing under
`/uml`, and BusyBox in `/bin` as the userland. `linux/amd64` only: mainline UML
has no arm64 port, and the image can't run emulated on arm64 hosts either (UML
controls its guest processes with `ptrace`, which QEMU user mode doesn't
implement).

The kernel is built in a stage on stock `ubuntu:24.04`. It is a tier-0 image —
it depends on nothing else in this repo, so tier-1 images can build `FROM` it.
Tags track the kernel release, pinned to the 6.18 LTS (`:6.18`) in
`version.sh`, plus `:latest`, rebuilt nightly.

## Usage

With no arguments you get a shell in the guest — the image's `CMD` is `sh`, as
in `ubuntu` or `alpine` (`exit` to leave; job control and `^C` work):

```sh
docker run --rm -it ghcr.io/cnuss/linux
```

Arguments are a command, joined with spaces and run in the guest with
`/bin/sh -c`; the container exits with its status. `/proc/cpuinfo` reports
`User Mode Linux`, not the host CPU:

```sh
docker run --rm ghcr.io/cnuss/linux uname -a
docker run --rm ghcr.io/cnuss/linux 'grep -E "vendor_id|model name" /proc/cpuinfo'
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

`--init` makes `docker stop` immediate. Without it the entrypoint script is
PID 1, which ignores `SIGTERM`, so Docker kills the container after its 10 s
grace period.

## Building on it

`ENTRYPOINT` and `SHELL` are the same wrapper, `/uml/bin/run`, so `RUN` in an
image built `FROM` this one runs inside the guest too, with the step's
`WORKDIR` and `ENV`, and the build fails if the command does. Whatever it
writes becomes part of that layer:

```dockerfile
FROM ghcr.io/cnuss/linux
WORKDIR /app
COPY . .
ENV MODE=release
RUN ./build.sh "$MODE"     # runs in the guest, in /app, with MODE set
```

Exec-form `RUN ["sh", "-c", ...]` runs in the guest too, through the
container's `/bin/sh` (below).

## `/bin/sh` and `/usr/bin/env`

The container's `/bin/sh` and `/usr/bin/env` are symlinks to `/uml/bin/run`,
which, invoked under those names, runs `sh` or `env` in the guest with exactly
the same arguments. So anything that reaches for a shell in the container
lands in the guest — `docker exec`, exec-form `RUN`, `#!/usr/bin/env`
scripts, health checks:

```sh
docker exec -it CONTAINER sh                     # a guest shell
docker exec CONTAINER sh -c 'uname -a'           # a command in a guest
```

Every call boots its own guest: `docker exec` doesn't join the guest the
container started with but boots another kernel on the same filesystem, so
files are shared and processes aren't. Inside a guest, `/bin/sh` and
`/usr/bin/env` are BusyBox's own — init bind-mounts BusyBox over
`/uml/bin/run` there, a mount the container never sees. Other commands in
`/bin` (`ls`, `cat`, ...) are BusyBox applets that `docker exec` runs directly
in the container, on the host kernel.

## How it runs

- **Root filesystem:** `root=/dev/root rootfstype=hostfs rootflags=/` — the
  guest's `/` is the container's `/`, with the guest's own `/dev`, `/proc` and
  `/sys` mounted over it. Guest writes are ordinary file writes: in the image
  layer during a build, in the container's layer at runtime (gone with `--rm`,
  kept by `docker commit`), or in a volume.
- **Handoff:** each call writes a small script to `/uml/tmp/run.<pid>` —
  export the caller's environment, `cd` to its working directory, `exec` the
  command — that the guest's init runs. Not the kernel command line, which
  mangles quotes and panics past 32 words. The guest reports the exit status
  through UML's `/proc/exitcode`.
- **Guest RAM:** UML backs its "physical memory" with a file in `$TMPDIR`
  that it maps executable, and refuses to boot if that directory is mounted
  `noexec` — which Docker's `/dev/shm` and any `--tmpfs` are by default. So
  the wrapper sets `TMPDIR=/uml/tmp` for UML alone; mounting a (noexec) tmpfs
  over `/tmp` doesn't affect it.
- **Console:** input comes from the container's stdin only when it is a
  terminal. UML hangs up a console whose input reaches EOF — and a build's
  stdin always has — after which every write to it fails.
- **Process tree:** `/uml/bin/run` stays the parent of UML rather than
  `exec`ing it, so UML is never PID 1: a guest kernel panic makes UML
  `abort()`, which a PID 1 would ignore and spin on forever.

## Build locally

```sh
docker build -t ghcr.io/cnuss/linux --build-arg VERSION="$(./0-linux/version.sh)" 0-linux/
docker run --rm ghcr.io/cnuss/linux uname -a
```
