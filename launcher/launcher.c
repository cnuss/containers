// Single-file program: this static binary carries a program and the files it
// needs (see pack.mjs), unpacks them once per uid into a private dir and execs
// the program from there.
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/sendfile.h>
#include <sys/stat.h>
#include <unistd.h>

#define TAIL_LEN 16
#define MAX_FILES 16
#define NAME_LEN 64

struct file {
    char name[NAME_LEN];
    unsigned long long size;
    off_t off;
};

static int self = -1;
static char id[65], exec_name[NAME_LEN], cafile[NAME_LEN];
static char interp_file[NAME_LEN], interp_loader[NAME_LEN];
static unsigned long long interp_off, interp_len;
static struct file files[MAX_FILES];
static int nfiles;

// Layout: files back to back, then the text header, then
// "PACK2 <header length, 9 digits>\n".
static int read_header(void) {
    char tail[TAIL_LEN + 1] = {0}, *header, *line, *save;
    unsigned long hlen;
    off_t end = lseek(self, 0, SEEK_END);
    if (pread(self, tail, TAIL_LEN, end - TAIL_LEN) != TAIL_LEN ||
        sscanf(tail, "PACK2 %9lu", &hlen) != 1 || hlen > 65536)
        return -1;
    if (!(header = calloc(1, hlen + 1)) ||
        pread(self, header, hlen, end - TAIL_LEN - hlen) != (ssize_t)hlen)
        return -1;
    off_t off = end - TAIL_LEN - hlen;
    for (line = strtok_r(header, "\n", &save); line; line = strtok_r(NULL, "\n", &save)) {
        if (sscanf(line, "file %63s %llu", files[nfiles].name, &files[nfiles].size) == 2) {
            if (++nfiles == MAX_FILES)
                return -1;
        } else if (sscanf(line, "interp %63s %63s %llu %llu", interp_file, interp_loader,
                          &interp_off, &interp_len) != 4 &&
                   sscanf(line, "id %64s", id) != 1 &&
                   sscanf(line, "exec %63s", exec_name) != 1 &&
                   sscanf(line, "cafile %63s", cafile) != 1) {
            return -1;
        }
    }
    for (int i = nfiles - 1; i >= 0; i--)
        off = files[i].off = off - files[i].size;
    free(header);
    return *id && *exec_name && nfiles > 0 && off >= 0 && interp_len <= 4096 ? 0 : -1;
}

static int copy_out(int out, off_t off, unsigned long long len) {
    while (len > 0) {
        ssize_t n = sendfile(out, self, &off, len > 1 << 30 ? 1 << 30 : len);
        if (n <= 0)
            return -1;
        len -= n;
    }
    return 0;
}

static int write_file(const char *tmp, const char *dir, const struct file *f) {
    char path[4096];
    snprintf(path, sizeof path, "%s/%s", tmp, f->name);
    int fd = open(path, O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC, 0700);
    if (fd < 0)
        return -1;
    int rc = copy_out(fd, f->off, f->size);
    if (rc == 0 && strcmp(f->name, interp_file) == 0) {
        // The interpreter slot is a fixed-size placeholder; zero-fill the tail
        char slot[4096] = {0};
        snprintf(slot, sizeof slot, "%s/%s", dir, interp_loader);
        if (pwrite(fd, slot, interp_len, interp_off) != (ssize_t)interp_len)
            rc = -1;
    }
    if (close(fd) != 0)
        rc = -1;
    return rc;
}

// Only a dir this uid owns and nobody else can write is trusted: the base
// dirs are shared and world-writable.
static int usable(const char *dir) {
    char path[4096];
    struct stat st;
    if (lstat(dir, &st) != 0 || !S_ISDIR(st.st_mode) || st.st_uid != getuid() ||
        (st.st_mode & 077) != 0)
        return 0;
    for (int i = 0; i < nfiles; i++) {
        snprintf(path, sizeof path, "%s/%s", dir, files[i].name);
        if (lstat(path, &st) != 0 || !S_ISREG(st.st_mode) ||
            (unsigned long long)st.st_size != files[i].size)
            return 0;
    }
    return 1;
}

static void remove_dir(const char *dir) {
    char path[4096];
    for (int i = 0; i < nfiles; i++) {
        snprintf(path, sizeof path, "%s/%s", dir, files[i].name);
        unlink(path);
    }
    rmdir(dir);
}

// Unpacks into a temp dir and renames it into place, so a dir at the final
// path is always complete; a concurrent run that renamed first wins.
static int unpack(const char *dir) {
    char tmp[4096];
    snprintf(tmp, sizeof tmp, "%s.XXXXXX", dir);
    if (!mkdtemp(tmp))
        return -1;
    for (int i = 0; i < nfiles; i++) {
        if (write_file(tmp, dir, &files[i]) != 0) {
            int saved = errno;
            remove_dir(tmp);
            errno = saved;
            return -1;
        }
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

// The packed CA bundle is only a fallback: a configured or system store wins
static void set_cafile(const char *dir) {
    static const char *system[] = {
        "/etc/ssl/certs/ca-certificates.crt", "/etc/pki/tls/certs/ca-bundle.crt",
        "/etc/ssl/ca-bundle.pem", "/etc/pki/tls/cacert.pem",
        "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", "/etc/ssl/cert.pem",
    };
    char path[4096];
    if (!*cafile || getenv("SSL_CERT_FILE") || getenv("SSL_CERT_DIR"))
        return;
    for (size_t i = 0; i < sizeof system / sizeof *system; i++)
        if (access(system[i], R_OK) == 0)
            return;
    snprintf(path, sizeof path, "%s/%s", dir, cafile);
    setenv("SSL_CERT_FILE", path, 1);
}

int main(int argc, char **argv) {
    (void)argc;
    self = open("/proc/self/exe", O_RDONLY | O_CLOEXEC);
    if (self < 0 || read_header() != 0) {
        fprintf(stderr, "launcher: can't read the packed payload from /proc/self/exe\n");
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
        // FROM scratch images ship no /tmp
        if (strcmp(bases[i], "/tmp") == 0 && mkdir("/tmp", 0700) == 0)
            chmod("/tmp", 01777);
        int n = snprintf(dir, sizeof dir, "%s/%s-%s-%u", bases[i], exec_name, id, getuid());
        if (*interp_file && n + 1 + strlen(interp_loader) >= interp_len) {
            errno = ENAMETOOLONG;
        } else if (usable(dir) || unpack(dir) == 0) {
            set_cafile(dir);
            snprintf(bin, sizeof bin, "%s/%s", dir, exec_name);
            execv(bin, argv);
            what = "run from";
        }
        used += snprintf(errs + used, sizeof errs - used, "%s: can't %s %s: %s\n",
                         exec_name, what, bases[i], strerror(errno));
        if (used >= sizeof errs)
            used = sizeof errs - 1;
    }
    fputs(errs, stderr);
    fprintf(stderr, "%s: set TMPDIR to a writable dir that allows exec\n", exec_name);
    return 127;
}
