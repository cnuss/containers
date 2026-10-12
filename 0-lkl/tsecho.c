/*
 * TCP echo server running in an LKL kernel, attached to a tailnet through
 * tsfifo. Usage: tsecho <tsfifo-dir> [port]
 *
 * Reads the node's tailnet address from tsfifo's config socket, brings up
 * the LKL "fifo" transport on ts.down/ts.up and listens on that address.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <lkl.h>
#include <lkl_host.h>

static int die(const char *what, long err)
{
	fprintf(stderr, "%s: %s\n", what, lkl_strerror(err));
	exit(1);
}

/* Wait until the config line says Running; copy the first IPv4 address. */
static void wait_for_tailnet(const char *path, char *ip, size_t ipsz)
{
	struct sockaddr_un sa = { .sun_family = AF_UNIX };
	char line[8192];
	size_t have = 0;
	int fd, i;

	snprintf(sa.sun_path, sizeof(sa.sun_path), "%s", path);
	for (i = 0; i < 100; i++) {
		fd = socket(AF_UNIX, SOCK_STREAM, 0);
		if (!connect(fd, (struct sockaddr *)&sa, sizeof(sa)))
			break;
		close(fd);
		fd = -1;
		usleep(100000);
	}
	if (fd < 0) {
		fprintf(stderr, "cannot connect to %s\n", path);
		exit(1);
	}

	for (;;) {
		char *nl, *p;
		ssize_t n = read(fd, line + have, sizeof(line) - 1 - have);

		if (n <= 0) {
			fprintf(stderr, "config socket closed\n");
			exit(1);
		}
		have += n;
		line[have] = 0;
		while ((nl = memchr(line, '\n', have))) {
			*nl = 0;
			fprintf(stderr, "tsfifo: %s\n", line);
			if (strstr(line, "\"state\":\"Running\"") &&
			    (p = strstr(line, "\"addresses\":[\""))) {
				p += strlen("\"addresses\":[\"");
				for (i = 0; p[i] && p[i] != '/' && p[i] != '"' &&
				     (size_t)i < ipsz - 1; i++)
					ip[i] = p[i];
				ip[i] = 0;
				close(fd);
				return;
			}
			have -= nl + 1 - line;
			memmove(line, nl + 1, have + 1);
		}
	}
}

int main(int argc, char **argv)
{
	const char *dir = argc > 1 ? argv[1] : "/tmp";
	int port = argc > 2 ? atoi(argv[2]) : 7000;
	char path[512], ifname[1100], ip[64];
	struct lkl_netdev *nd;
	struct lkl_netdev_args args = { 0 };
	struct lkl_sockaddr_in sa;
	int id, ifindex, ls, c, one = 1;
	long n;
	char buf[2048];

	snprintf(path, sizeof(path), "%s/ts-config.sock", dir);
	wait_for_tailnet(path, ip, sizeof(ip));
	printf("tailnet address: %s\n", ip);

	/* we read what tsfifo sends down, and write to its up FIFO */
	snprintf(ifname, sizeof(ifname), "%s/ts.down|%s/ts.up", dir, dir);
	nd = lkl_netdev_fifo_create(ifname);
	if (!nd)
		return 1;

	lkl_host_ops.print = NULL;
	lkl_init(&lkl_host_ops);
	id = lkl_netdev_add(nd, &args);
	if (id < 0)
		die("lkl_netdev_add", id);
	if ((n = lkl_start_kernel("mem=32M loglevel=3")))
		die("lkl_start_kernel", n);

	ifindex = lkl_netdev_get_ifindex(id);
	if ((n = lkl_if_up(ifindex)))
		die("lkl_if_up", n);
	if ((n = lkl_if_set_mtu(ifindex, LKL_NETDEV_FIFO_DEFAULT_MTU)))
		die("lkl_if_set_mtu", n);
	/*
	 * /10 makes the whole tailnet range (100.64.0.0/10) on-link; tsfifo
	 * answers ARP for every address, so no gateway is needed.
	 */
	if ((n = lkl_if_set_ipv4(ifindex, inet_addr(ip), 10)))
		die("lkl_if_set_ipv4", n);

	ls = lkl_sys_socket(LKL_AF_INET, LKL_SOCK_STREAM, 0);
	if (ls < 0)
		die("socket", ls);
	lkl_sys_setsockopt(ls, LKL_SOL_SOCKET, LKL_SO_REUSEADDR,
			   (char *)&one, sizeof(one));
	memset(&sa, 0, sizeof(sa));
	sa.sin_family = LKL_AF_INET;
	sa.sin_port = htons(port);
	sa.sin_addr.lkl_s_addr = inet_addr(ip);
	if ((n = lkl_sys_bind(ls, (struct lkl_sockaddr *)&sa, sizeof(sa))))
		die("bind", n);
	if ((n = lkl_sys_listen(ls, 8)))
		die("listen", n);
	printf("echo server listening on %s:%d (inside LKL, via tailnet)\n",
	       ip, port);
	fflush(stdout);

	for (;;) {
		c = lkl_sys_accept(ls, NULL, NULL);
		if (c < 0)
			die("accept", c);
		printf("client connected\n");
		fflush(stdout);
		while ((n = lkl_sys_read(c, buf, sizeof(buf))) > 0) {
			printf("echoing %ld bytes\n", n);
			fflush(stdout);
			lkl_sys_write(c, buf, n);
		}
		lkl_sys_close(c);
	}
}
