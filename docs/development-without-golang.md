# Development Without Go

You can build, test and package Convoy without needing a Linux machine with dependencies.  On a Mac for example,
create a docker-machine that you can mount your host file system into.  Then

```bash
docker build -t convoy -f Dockerfile.dapper .
```

## Build

```bash
docker run --rm -v $(pwd):/go/src/github.com/rancher/convoy -it convoy
```

## Test

```bash
docker run --rm -v $(pwd):/go/src/github.com/rancher/convoy -it convoy test
```

## Package

```bash
docker run --rm -v $(pwd):/go/src/github.com/rancher/convoy -it convoy package
```

After running `package`, you will have "dist/artifacts/convoy.tar.gz" which you can install
by following the directions in the main README.md.

