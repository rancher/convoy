# Convoy [![Build Status](http://ci.rancher.io/api/badge/github.com/rancher/convoy/status.svg?branch=master)](http://ci.rancher.io/github.com/rancher/convoy)

# Overview
Convoy is a generic Docker volume plugin for a variety fo storage back-ends. It's designed to simply implmentation of Docker volume plugins while supporting vendor-specific extensions such as snapshots, backups and restore. It's written in Go and can be deployed as a simple standalone binary.

# Usage

## Install
1. Download latest version of [convoy](https://github.com/rancher/convoy/releases/download/v0.2-rc4/convoy) binary and put it in your $PATH(e.g. /usr/local/bin).
2. Download latest version of [thin-provisioning-tools](https://github.com/rancher/thin-provisioning-tools/releases/download/convoy-v0.2/pdata_tools) binary and put it in your $PATH(e.g. /usr/local/bin) as well. It's a Rancher Labs maintained version of thin-provisioning-tools to work with device mapper driver.
3. Notice: please make sure $PATH can be access by sudo user or root.

## Build

If you prefer building it by yourself:

1. Environment: Require Go environment, mercurial and libdevmapper-dev package.
2. Use docker v1.8+, which supports volume plugins in stable version.
3. Install thin-provisioning-tools according to the instruction above.
4. Build and install:
```
go get github.com/rancher/convoy
cd $GOPATH/src/github.com/rancher/convoy
make
sudo make install
```
The last line would install convoy to /usr/local/bin/, otherwise executables are
at bin/ directory.

## Setup

### Stop Docker daemon
Normally you need to do:
```
sudo service docker stop
```
### Install plugin to Docker (apply to v1.8+)
```
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

### Start Convoy server

Convoy supports different drivers, and can be easily extended. Currently it contains two implementations of driver: VFS, or device mapper. EBS support is coming.

#### VFS driver
##### Choose a directory as root to store the volumes
We would refer the directory as ```<vfs_path>``` below.
##### Start server
```
sudo convoy server --drivers vfs --driver-opts vfs.path=<vfs_path>
```

#### Device mapper driver
convoy can work with any type of block devices, we show cases two kinds of setup here: new LVM logical volumes, or loopbacks. You can use or create your own block devices in your preferred way as well.
##### Prepare two block devices for device mapper storage pool
###### Create two block devices using LVM2 for device mapper storage pool:
Assuming the volume group ```convoy-vg``` already exists, and you want to create a 100G pool out of it. Please refer to http://tldp.org/HOWTO/LVM-HOWTO/createlv.html for more details on creating logical volume using LVM2.
```
sudo lvcreate -L 100000 -n volume-data convoy-vg
sudo lvcreate -L 1000 -n volume-metadata convoy-vg
```
The devices would be called ```<datadev>```(``/dev/convoy-vg/volume-data```) and ```<metadatadev>``` (```/dev/convoy-vg/volume-metadata```) respectively below.

###### (Alternative) Create two loopback devices for device mapper storage pool:
```
truncate -s 100G data.vol
truncate -s 1G metadata.vol
sudo losetup -f data.vol
sudo losetup -f metadata.vol
```
The devices would be called ```<datadev>```(e.g. ```/dev/loop0```) and ```<metadatadev>``` (e.g. ```/dev/loop1```) respectively below.

##### Start server
```
sudo convoy server --drivers devicemapper --driver-opts dm.datadev=<datadev> --driver-opts dm.metadatadev=<metadatadev>
```
* Device mapper default volume size is 100G. You can override it with e.g. "--driver-opts dm.defaultvolumesize=10G"

#### Start Docker server
Normally you need to do:
```
sudo service docker start
```
#### Test run
```
sudo docker -it test_volume:/test --volume-driver=convoy ubuntu /bin/bash
```
#### Tips
1. As long as convoy has been initialized once, next time "convoy server" would be enough to start it.
2. The server metadata files would be stored at /var/lib/convoy by default.
3. Different drivers can co-exists at the same time, just add "--drivers" and "--driver-opts" to start command line.
4. The driver can be chosen though "convoy create <name> --driver" command. The first driver in the "--drivers" list would be default driver, and would be used if volume created without specified driver.

## Use cases
##### Create a volume:
```
sudo docker run -it -v vol1:/vol1 --volume-driver=convoy ubuntu /bin/bash
```
or
```
sudo convoy create vol1 --size 10G
```

##### Create volume using different drivers:
```
sudo convoy create vol1 --driver vfs
```
* ```--size``` option has no effect on vfs.

##### List/inspect volumes:
```
sudo convoy list
sudo convoy inspect vol1
```

##### Take snapshot of volume:
```
sudo convoy snapshot create vol1 --name snap1vol1
```

##### Backup the snapshot to S3 or local filesystem(can be nfs mounted):
```
sudo convoy backup create snap1vol1 --dest s3://backup-bucket@us-west-2/
```
or
```
sudo convoy backup create snap1vol1 --dest vfs:///opt/backup/
```

It would return a url like
```
s3://backup-bucket@us-west-2/?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0
```
or
```
vfs:///opt/backup?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0
```
We would refer it as <url> below.
* For S3, please make sure you have AWS credential ready either at ```~/.aws/credentials``` or as environment variables, as described (here)[http://blogs.aws.amazon.com/security/post/Tx3D6U6WSFGOK2H/A-New-and-Standardized-Way-to-Manage-Credentials-in-the-AWS-SDKs]. You may need to put credentials to ```/root/.aws/credentials``` or setup sudo environment variables in order to get S3 credential works.

##### Create a new volume using the backup
```
sudo convoy create res1 --backup <url>
```

##### Use the new volume with docker
```
sudo docker run -it -v res1:/res1 --volume-driver convoy ubuntu /bin/bash
```

## Tips
1. ```--help``` can be helpful most of time.
2. Volumes/Snapshots can be referred by either name, full UUID or partial(shorthand) UUID
3. Name is not mandatory. If left empty, you can refer to it by UUID or partial UUID.
