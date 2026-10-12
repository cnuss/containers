/*
 * runas UID GID GROUPS COMMAND [ARG...]
 *
 * Runs COMMAND with the given numeric uid, gid and comma-separated
 * supplementary groups. BusyBox has nothing that switches to a bare uid
 * without an /etc/passwd entry.
 */
#include <errno.h>
#include <grp.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static unsigned long number(const char *s)
{
	char *end;
	unsigned long n;

	errno = 0;
	n = strtoul(s, &end, 10);
	if (errno || end == s || *end) {
		fprintf(stderr, "runas: bad id: %s\n", s);
		exit(126);
	}
	return n;
}

int main(int argc, char **argv)
{
	gid_t groups[NGROUPS_MAX];
	size_t ngroups = 0;
	char *g;

	if (argc < 5) {
		fprintf(stderr, "usage: runas UID GID GROUPS COMMAND [ARG...]\n");
		return 2;
	}
	for (g = strtok(argv[3], ","); g && ngroups < NGROUPS_MAX; g = strtok(NULL, ","))
		groups[ngroups++] = number(g);

	if (setgroups(ngroups, groups) || setgid(number(argv[2])) || setuid(number(argv[1]))) {
		perror("runas");
		return 126;
	}
	execvp(argv[4], argv + 4);
	fprintf(stderr, "runas: %s: %s\n", argv[4], strerror(errno));
	return errno == ENOENT ? 127 : 126;
}
