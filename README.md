# rancher-volume [![BuildStatus](http://ci.rancher.io/api/badge/github.com/rancher/rancher-volume/status.svg?branch=master)](http://ci.rancher.io/github.com/rancher/rancher-volume)

# Overview
Rancher-volume is a storage driver platform can be integrated with Docker,
managing docker volumes.

# Features
Main feature of rancher-volume:
1. Integration with Docker.
2. Implements device mapper storage driver, make it possible to use device
   mapper with docker volumes.
3. Take snapshot of volume, back it up to S3 or local disk/nfs.

# Usage

## Build

1. Environment: Require Go environment, mercurial and libdevmapper-dev package.
2. Install [thin-provisioning-tools](https://github.com/rancher/thin-provisioning-tools.git)
* It's a Rancher Labs maintained version of thin-provisioning-tools, ensure
the compatibility of rancher-volume.
3. Use docker v1.7.x expermential binary. Can be found at https://blog.docker.com/2015/06/experimental-binary/
* docker would include volume plugin in master in v1.8
3. Build:
```
go get github.com/rancher/rancher-volume
cd $GOPATH/src/github.com/rancher/rancher-volume
make
```
4. Install:
```
sudo make install
```
This would install rancher-volume to /usr/local/bin/, otherwise executables are
at bin/ directory.

## Setup

rancher-volume can work with any type of block devices, we show cases a loopback
based device here for tutorial.

1. Create two loopback devices for device mapper storage pool:
```
truncate -s 100G data.vol
truncate -s 1G metadata.vol
sudo losetup -f data.vol
sudo losetup -f metadata.vol
```
The devices would be called /dev/loop0 and /dev/loop1 respectively below.
2. Place hook to Docker(apply to Docker v1.7.x experimental)
```
echo "unix:///var/run/rancher/volume.sock” > /usr/share/docker/plugins/rancher.spec
```
3. Setup server
```
sudo rancher-volume server --driver-opts dm.datadev=/dev/loop0 --driver-opts dm.metadatadev=/dev/loop1
```
* As long as rancher-volume has been initialized once, next time "rancher-volume server" would be enough to start it
* The server configuration file would be at /var/lib/rancher-volume by default.
4. Start Docker.

## Use cases
1. Create a volume:
```
sudo docker run -it -v vol1:/vol1 --volume-driver rancher ubuntu /bin/bash
```
or
```
sudo rancher-volume create vol1 --size 10G
```
2. List/inspect volumes:
```
sudo rancher-volume list
sudo rancher-volume inspect vol1
```
3. Take snapshot of volume:
```
sudo rancher-volume snapshot create vol1 --name snap1vol1
```
4. Backup the snapshot to S3 or local filesystem(can be nfs mounted):
```
sudo rancher-volume backup create snap1vol1 --dest s3://backup-bucket@us-west-2/
```
or
```
sudo rancher-volume backup create snap1vol1 --dest vfs:///opt/backup/
```
It would return a url like [this](s3://backup-bucket@us-west-2/?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0)
or [this](vfs:///opt/backup?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0)
5. Create a new volume using the backup
```
sudo rancher-volume create res1 --backup <url>
```
6. Use the new volume with docker
```
sudo docker run -it -v res1:/res1 --volume-driver rancher ubuntu /bin/bash
```

## Tips
1. "--help" would be helpful most of time.
2. Volume/Snapshot can be referred by either name, full UUID or partial(shorthand) UUID
3. Name is not mandatory now. If left empty, you can refer to it by UUID or partial UUID.
4. AWS credentials are provided through normal way, e.g. ~/.aws/credentials. We didn't store credentials.
