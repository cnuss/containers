// Single-file claude: this static binary carries the musl loader and the
// claude ELF (see pack.mjs), unpacks them once per uid into a private dir and
// execs claude from there.
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/sendfile.h>
#include <sys/stat.h>
#include <unistd.h>

#define TRAILER_LEN 128

static int self = -1;
static char id[65];
static unsigned long long ld_size, claude_size, interp_off, interp_len;

static int copy_out(int out, off_t off, unsigned long long len) {
    while (len > 0) {
        ssize_t n = sendfile(out, self, &off, len > 1 << 30 ? 1 << 30 : len);
        if (n <= 0)
            return -1;
        len -= n;
    }
    return 0;
}

static int write_file(const char *dir, const char *name, off_t off,
                      unsigned long long len, const char *interp) {
    char path[4096];
    snprintf(path, sizeof path, "%s/%s", dir, name);
    int fd = open(path, O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC, 0700);
    if (fd < 0)
        return -1;
    int rc = copy_out(fd, off, len);
    if (rc == 0 && interp) {
        // The interpreter slot is a fixed-size placeholder; zero-fill the tail
        char slot[4096] = {0};
        strcpy(slot, interp);
        if (pwrite(fd, slot, interp_len, interp_off) != (ssize_t)interp_len)
            rc = -1;
    }
    if (close(fd) != 0)
        rc = -1;
    return rc;
}

static int has_file(const char *dir, const char *name, unsigned long long size) {
    char path[4096];
    struct stat st;
    snprintf(path, sizeof path, "%s/%s", dir, name);
    return lstat(path, &st) == 0 && S_ISREG(st.st_mode) &&
           (unsigned long long)st.st_size == size;
}

// Only a dir this uid owns and nobody else can write is trusted: the base
// dirs are shared and world-writable.
static int usable(const char *dir) {
    struct stat st;
    return lstat(dir, &st) == 0 && S_ISDIR(st.st_mode) &&
           st.st_uid == getuid() && (st.st_mode & 077) == 0 &&
           has_file(dir, "ld.so", ld_size) &&
           has_file(dir, "claude", claude_size);
}

static void remove_dir(const char *dir) {
    char path[4096];
    snprintf(path, sizeof path, "%s/ld.so", dir);
    unlink(path);
    snprintf(path, sizeof path, "%s/claude", dir);
    unlink(path);
    rmdir(dir);
}

// Unpacks into a temp dir and renames it into place, so a dir at the final
// path is always complete; a concurrent run that renamed first wins.
static int unpack(const char *dir) {
    char tmp[4096], interp[4096];
    snprintf(tmp, sizeof tmp, "%s.XXXXXX", dir);
    snprintf(interp, sizeof interp, "%s/ld.so", dir);
    if (!mkdtemp(tmp))
        return -1;
    off_t payload = lseek(self, 0, SEEK_END) - TRAILER_LEN - ld_size - claude_size;
    if (write_file(tmp, "ld.so", payload, ld_size, NULL) != 0 ||
        write_file(tmp, "claude", payload + ld_size, claude_size, interp) != 0) {
        int saved = errno;
        remove_dir(tmp);
        errno = saved;
        return -1;
    }
    if (rename(tmp, dir) == 0)
        return 0;
    int saved = errno;
    remove_dir(tmp);
    if (usable(dir))
        return 0;
    errno = saved;
    return -1;
}

int main(int argc, char **argv) {
    (void)argc;
    char trailer[TRAILER_LEN + 1] = {0};
    self = open("/proc/self/exe", O_RDONLY | O_CLOEXEC);
    if (self < 0 ||
        pread(self, trailer, TRAILER_LEN, lseek(self, 0, SEEK_END) - TRAILER_LEN) != TRAILER_LEN ||
        sscanf(trailer, "CLAUDEPACK1 %64s %llu %llu %llu %llu", id, &ld_size,
               &claude_size, &interp_off, &interp_len) != 5 ||
        interp_len > 4096) {
        fprintf(stderr, "claude: can't read the packed payload from /proc/self/exe\n");
        return 127;
    }

    // Errors are only reported once every base dir has failed
    char errs[8192] = {0};
    size_t used = 0;
    const char *tmpdir = getenv("TMPDIR");
    const char *bases[] = {tmpdir && *tmpdir ? tmpdir : "/tmp", "/tmp", "/dev/shm"};
    for (size_t i = 0; i < sizeof bases / sizeof *bases; i++) {
        if (i > 0 && strcmp(bases[i], bases[0]) == 0)
            continue;
        char dir[4096], bin[4096];
        const char *what = "unpack into";
        int n = snprintf(dir, sizeof dir, "%s/claude-%s-%u", bases[i], id, getuid());
        if (n + sizeof "/ld.so" > interp_len) {
            errno = ENAMETOOLONG;
        } else if (usable(dir) || unpack(dir) == 0) {
            snprintf(bin, sizeof bin, "%s/claude", dir);
            execv(bin, argv);
            what = "run from";
        }
        used += snprintf(errs + used, sizeof errs - used, "claude: can't %s %s: %s\n",
                         what, bases[i], strerror(errno));
        if (used >= sizeof errs)
            used = sizeof errs - 1;
    }
    fputs(errs, stderr);
    fputs("claude: set TMPDIR to a writable dir that allows exec\n", stderr);
    return 127;
}
