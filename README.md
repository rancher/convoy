# Convoy [![Build Status](http://ci.rancher.io/api/badge/github.com/rancher/convoy/status.svg?branch=master)](http://ci.rancher.io/github.com/rancher/convoy)

# Overview
Convoy is a generic Docker volume plugin for a variety of storage back-ends. It's designed to simplify the implementation of Docker volume plug-ins while supporting vendor-specific extensions such as snapshots, backups and restore. It's written in Go and can be deployed as a simple standalone binary.

# Usage

## Requirement:
1. Docker v1.8+, which supports volume plugins in stable version.
2. Download thin-provisioning-tools from [thin-provisioning-tools](https://github.com/rancher/thin-provisioning-tools/releases/download/convoy-v0.2/pdata_tools), then put it in your ```$PATH```(e.g. /usr/local/bin). It's a Rancher Labs maintained version of thin-provisioning-tools to work with device mapper driver. Notice: please make sure ```$PATH``` can be access by sudo user or root.

## Install
Download latest version of [convoy](https://github.com/rancher/convoy/releases/download/v0.2-rc5/convoy) binary and put it in your ```$PATH```(e.g. /usr/local/bin). Notice: please make sure ```$PATH``` can be access by sudo user or root.

## Setup

### Install plugin to Docker (apply to v1.8+)
```
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

### Start Convoy server

Convoy supports different drivers, and can be easily extended. Currently it contains two driver implementations: VFS/NFS, or device mapper. EBS support is coming.

#### VFS/NFS driver
##### Choose a directory as root to store the volumes
1. Create a ```<vfs_path>``` as you chose.
2. NFS: if you want to use NFS, then mount your NFS to ```<vfs_path>``` as you chose. e.g.
```
sudo mkdir /opt/nfs
sudo mount -t nfs 1.2.3.4:/path /opt/nfs
```
Below we would refer ```/opt/nfs``` as ```<vfs_path>```

##### Start server
```
sudo convoy server --drivers vfs --driver-opts vfs.path=<vfs_path>
```

#### Device mapper driver
Convoy can work with any type of block devices, we show two kinds of setup here: new LVM logical volumes, or [loopbacks](docs/loopback.md). You can use or create your own block devices in your preferred way.
##### Prepare two block devices for device mapper storage pool
###### Create two block devices using LVM2 for device mapper storage pool:
Assuming the volume group ```convoy-vg``` already exists, and you want to create a 100G pool out of it. Please refer to http://tldp.org/HOWTO/LVM-HOWTO/createlv.html for more details on creating logical volume using LVM2.
```
sudo lvcreate -L 100000 -n volume-data convoy-vg
sudo lvcreate -L 1000 -n volume-metadata convoy-vg
```
The devices would be called ```<datadev>```(``/dev/convoy-vg/volume-data```) and ```<metadatadev>``` (```/dev/convoy-vg/volume-metadata```) respectively below.

##### Start server
```
sudo convoy server --drivers devicemapper --driver-opts dm.datadev=<datadev> --driver-opts dm.metadatadev=<metadatadev>
```
* Device mapper default volume size is 100G. You can override it with e.g. ```--driver-opts dm.defaultvolumesize=10G```

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
###### Docker
```
sudo docker run -it -v volume_name:/volume --volume-driver=convoy ubuntu /bin/bash
```
* If a volume with the name```volume_name``` has already been created by Convoy before ```docker run```, Docker would use that volume directly, rather than create a new one with that name again. Duplicate names is not allow locally.

###### Convoy
```
sudo convoy create volume_name
```
or
```
sudo convoy create volume_name --driver <driver>
```
* ```--size``` option is available for device mapper driver.

###### Convoy VFS/NFS: Reuse the existing volume in NFS server:
In order to reuse the existing volume in NFS server, just use the same name when you create the volume in the new node:
E.g. on VM1, with VFS as default driver, we want to create volume ```vol1```:
```
user@vm1:~$ sudo docker run -it -v vol1:/vol1 --volume-driver=convoy ubuntu /bin/bash
```
Later, on VM2, with VFS/NFS driver's vfs.path pointed to the same NFS directory as VM1, but VFS is not default driver. We want to reuse ```vol1```:
```
user@vm2:~$ sudo convoy create vol1 --driver vfs
user@vm2:~$ sudo docker run -it -v vol1:/new_vol1 --volume-driver=convoy ubuntu /bin/bash
```
##### Delete a volume:
###### Docker(delete volume with container):
```
sudo docker rm -v <container_name>
```
* Notice if Convoy default driver is VFS, ```docker rm -v``` wouldn't remove volume from hard disk. You need to use ```convoy delete <volume_name> --cleanup``` stated below.

###### Convoy
```
sudo convoy delete <volume_name>
```
* ```--cleanup``` option is available for VFS/NFS driver. Volume on the disk for VFS/NFS driver would only been deleted if ```--cleanup``` is specified.

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
We would refer it as ```<url>``` below.
* For S3, please make sure you have AWS credential ready either at ```~/.aws/credentials``` or as environment variables, as described [here](http://blogs.aws.amazon.com/security/post/Tx3D6U6WSFGOK2H/A-New-and-Standardized-Way-to-Manage-Credentials-in-the-AWS-SDKs). You may need to put credentials to ```/root/.aws/credentials``` or setup sudo environment variables in order to get S3 credential works.

##### Create a new volume using the backup
```
sudo convoy create res1 --backup <url>
```

##### Use the new volume with docker
```
sudo docker run -it -v res1:/res1 --volume-driver convoy ubuntu /bin/bash
```

## Tips
1. ```--help``` can be helpful.
2. When working with Convoy CLI, volumes/Snapshots can be referred by either name, full UUID or partial(shorthand) UUID. Docker can only refer to volume by volume's name.
3. You can create a volume without name. In this case, a name would be automatically generated for you, based on UUID.

## Build

If you prefer building it by yourself:

1. Environment: Require Go environment, mercurial and libdevmapper-dev package.
2. Build and install:
```
go get github.com/rancher/convoy
cd $GOPATH/src/github.com/rancher/convoy
make
sudo make install
```
The last line would install convoy to /usr/local/bin/, otherwise executables are
at bin/ directory.
