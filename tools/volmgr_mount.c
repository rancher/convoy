#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <sched.h>
#include <unistd.h>
#include <fcntl.h>
#include <string.h>

int main(int argc, char *argv[])
{
	int fd;

	if (argc < 3) {
		fprintf(stderr, "%s <mount_namespace_fd> <-m|-u> <mount_args>", argv[0]);
		exit(-1);
	}

	fd = open(argv[1], O_RDONLY);
	if (fd == -1) {
		perror("open mount namespace fd failed");
		exit(-1);
	}

	if (setns(fd, 0) != 0) {
		perror("failed to switch namespace");
		exit(-1);
	}

	if (strncmp(argv[2], "-m", 2) == 0) {
		argv[2] = "mount";
		if (execvp("mount", &argv[2]) != 0) {
			perror("mount failed");
			exit(-1);
		}
	} else if (strncmp(argv[2], "-u", 2) == 0) {
		argv[2] = "umount";
		if (execvp("umount", &argv[2]) != 0) {
			perror("umount failed");
			exit(-1);
		}
	} else {
		fprintf(stderr, "unrecognize parameter!");
		exit(-1);
    }
}
