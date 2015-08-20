# Convoy [![Build Status](http://ci.rancher.io/api/badge/github.com/rancher/convoy/status.svg?branch=master)](http://ci.rancher.io/github.com/rancher/convoy)

# Overview
Convoy is a generic Docker volume plugin for a variety of storage back-ends. It's designed to simplify the implementation of Docker volume plug-ins while supporting vendor-specific extensions such as snapshots, backups and restore. It's written in Go and can be deployed as a simple standalone binary.

# TL; DR (a.k.a Quick Hands-on)
Check Docker version. Make sure it's 1.8+
```
docker --version
```
Otherwise you need to upgrade:
```
curl -sSL https://get.docker.com/ | sh
```
Quick development server setup(WARNING: NOT FOR PRODUCTION BECAUSE LOOPBACK IS NOT RECOMMENDED)
```
wget https://github.com/rancher/convoy/releases/download/v0.2-rc6/convoy.tar.gz
tar xvf convoy.tar.gz
sudo cp convoy/* /usr/local/bin/
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
truncate -s 100G data.vol
truncate -s 1G metadata.vol
sudo losetup /dev/loop5 data.vol
sudo losetup /dev/loop6 metadata.vol
sudo convoy server --drivers devicemapper --driver-opts dm.datadev=/dev/loop5 --driver-opts dm.metadatadev=/dev/loop6
```
Create volume/snapshot/backup:
```
sudo mkdir /opt/convoy/
sudo docker run -v vol1:/vol1 --volume-driver=convoy ubuntu touch /vol1/foo
sudo convoy snapshot create vol1 --name snap1vol1
sudo convoy backup create snap1vol1 --dest vfs:///opt/convoy/
```
The last command returned ```<backup_url>```, then:
```
sudo convoy create res1 --backup <backup_url>
sudo docker run -v res1:/res1 --volume-driver=convoy ubuntu ls /res1/foo
```

# Usage

## Requirement:
Docker v1.8+, which supports volume plugins in stable version.

## Install
Download latest version of [convoy](https://github.com/rancher/convoy/releases/download/v0.2-rc6/convoy.tar.gz) and unzip it. Put the binaries in your ```$PATH```(e.g. /usr/local/bin). Notice: please make sure ```$PATH``` can be access by sudo user or root. E.g.
```
wget https://github.com/rancher/convoy/releases/download/v0.2-rc6/convoy.tar.gz
tar xvf convoy.tar.gz
sudo cp convoy/* /usr/local/bin/
```

## Setup

### Install plugin to Docker
```
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

### Start Convoy server

Convoy supports different drivers, and can be easily extended. Currently it contains two driver implementations: VFS/NFS, or device mapper. EBS support is coming.

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

#### Test run
```
sudo docker -it test_volume:/test --volume-driver=convoy ubuntu /bin/bash
```

## Command examples:
##### Create a volume:
```
sudo docker run -it -v volume_name:/volume --volume-driver=convoy ubuntu /bin/bash
```
* If a volume with the name```volume_name``` has already been created by Convoy before ```docker run```, Docker would use that volume directly, rather than create a new one with that name again. Duplicate names is not allow locally.

or
```
sudo convoy create volume_name
```
* ```--size``` option is available for device mapper driver.

##### Delete a volume:
```
sudo docker rm -v <container_name>
```
or
```
sudo convoy delete <volume_name>
```
* ```--reference``` option is available for VFS/NFS driver for ```convoy delete```. Volume on the disk for VFS/NFS driver wouldn't be deleted if ```--reference``` is specified.

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

##### Reuse the existing volume in NFS server:
In order to reuse the existing volume in NFS server, just use the same name when you create the volume in the new node:
E.g. on VM1, with VFS as default driver, we want to create volume ```vol1```:
```
user@vm1:~$ sudo docker run -it -v vol1:/vol1 --volume-driver=convoy ubuntu /bin/bash
```
Later, on VM2:
```
user@vm2:~$ sudo docker run -it -v vol1:/new_vol1 --volume-driver=convoy ubuntu /bin/bash
```
Then VM1 and VM2 shared the same volume.

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
