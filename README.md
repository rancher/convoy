# Convoy [![Build Status](https://drone.rancher.io/api/badges/rancher/convoy/status.svg)](https://drone.rancher.io/rancher/convoy)

## Overview
Convoy is a  Docker volume plugin for a variety of storage back-ends. It's designed to be a simple Docker volume plug-ins that supports vendor-specific extensions such as snapshots, backups and restore. It's written in Go and can be deployed as a standalone binary.

[![Convoy_DEMO](https://asciinema.org/a/9y5nbp3h97vyyxnzuax9f568e.png)](https://asciinema.org/a/9y5nbp3h97vyyxnzuax9f568e?autoplay=1&loop=1&size=medium&speed=2)

## Why we need Convoy?
Docker has various drivers(aufs, device mapper, etc) for container's root image, but not for volumes. User can create volume through ```docker run -v volname```, but it's disposable, cannot be easily reused for new containers or containers on the other hosts. For example, if you start a wordpress container with database, add some posts, remove the container, then the modified database would lost.

Before volume plugin, the only way to reuse the volume is using host bind mount feature of Docker, as ```docker run -v /host_path:/container_path```, then maintain the content of the volume at ```/host_path```. You can also use ```--volume-from``` but that would require original container still exists on the same host.

Convoy used Docker volume plugin mechanism to provide persistent volume for Docker containers, and supports various of backends(e.g. device mapper, NFS, EBS) and more features like snapshot/backup/restore. So user would able to migrate the volumes between the hosts, share the same volume across the hosts, make scheduled snapshots of as well as recover to previous version of volume. It's much easier for user to manage data with Docker volumes with Convoy.

### Backends supported by Convoy currently
* Device Mapper
* Virtual File System(VFS)/Network File System(NFS)
* Amazon Elastic Block Store(EBS)

## Quick Start Guide
First let's make sure we have Docker 1.8 or above running.
```
docker --version
```
If not, install the latest Docker daemon as follows:
```
curl -sSL https://get.docker.com/ | sh
```
Once we have made sure we have the right Docker daemon running, we can install and configure Convoy volume plugin as follows:
```
wget https://github.com/rancher/convoy/releases/download/v0.5.0/convoy.tar.gz
tar xvf convoy.tar.gz
sudo cp convoy/convoy convoy/convoy-pdata_tools /usr/local/bin/
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```
We can use file-backed loopback device to test and demo Convoy Device Mapper driver. Loopback device, however, is known to be unstable and should not be used in production.
```
truncate -s 100G data.vol
truncate -s 1G metadata.vol
sudo losetup /dev/loop5 data.vol
sudo losetup /dev/loop6 metadata.vol
```
Once we have the data and metadata device setup, we can start the Convoy plugin daemon as follows:
```
sudo convoy daemon --drivers devicemapper --driver-opts dm.datadev=/dev/loop5 --driver-opts dm.metadatadev=/dev/loop6
```
We can create a Docker container with a convoy volume. As a test, we create a file called `/vol1/foo` in the convoy volume:
```
sudo docker run -v vol1:/vol1 --volume-driver=convoy ubuntu touch /vol1/foo
```
Next we take a snapshot of the convoy volume. We backup the snapshot to a local directory: (Backup to NFS share or S3 objectore is also supported.)
```
sudo convoy snapshot create vol1 --name snap1vol1
sudo mkdir -p /opt/convoy/
sudo convoy backup create snap1vol1 --dest vfs:///opt/convoy/
```
The `convoy backup` command returns a URL string representing backup dataset. You can use the same URL string to recover the volume to another host:
```
sudo convoy create res1 --backup <backup_url>
```
The following command creates a new container and mounts the recovered convoy volume into that container:
```
sudo docker run -v res1:/res1 --volume-driver=convoy ubuntu ls /res1/foo
```
You should see the recovered file ```/res1/foo```.

## Installation
Ensure you have Docker 1.8 or above installed.

Download latest version of [convoy](https://github.com/rancher/convoy/releases/download/v0.5.0/convoy.tar.gz) and unzip it. Put the binaries in a directory in the execution ```$PATH``` of sudo and root users (e.g. /usr/local/bin).
```
wget https://github.com/rancher/convoy/releases/download/v0.5.0/convoy.tar.gz
tar xvf convoy.tar.gz
sudo cp convoy/convoy convoy/convoy-pdata_tools /usr/local/bin/
```
Run the following commands to setup the Convoy volume plugin for Docker:
```
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

## Start Convoy Daemon

You need to pass different arguments to convoy daemon depending on the choice of backend implementation.

#### Device Mapper
For production environment, it's recommended to attach a new empty block device to the host Convoy running on for using Device Mapper driver. Then you can make two partitions out of the device using [`dm_dev_partition.sh`](https://raw.githubusercontent.com/rancher/convoy/master/tools/dm_dev_partition.sh) to get two block devices fitting for Device Mapper driver. See [Device Mapper Partition Helper](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#device-mapper-partition-helper) for more details.

Device Mapper requires two block devices to create storage pool for all volumes and snapshots. Assuming you have two devices created, one data device called `/dev/convoy-vg/data` and the other metadata device called `/dev/convoy-vg/metadata`. You run the following command to start the Convoy daemon:
```
sudo convoy daemon --drivers devicemapper --driver-opts dm.datadev=/dev/convoy-vg/data --driver-opts dm.metadatadev=/dev/convoy-vg/metadata
```
* A default Device Mapper volume size is 100G. You can override it with the `---driver-opts dm.defaultvolumesize` option.
* You can take a look at [here](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#calculate-the-size-you-need-for-metadata-block-device) if you want to know how much storage need to be allocated for metadata device.

#### NFS
First, mount the NFS share to the root directory used to store volumes. Substitute `<vfs_path>` to the appropriate directory of your choice:
```
sudo mkdir <vfs_path>
sudo mount -t nfs <nfs_server>:/path <vfs_path>
```
The NFS-based Convoy daemon can be started as follows:
```
sudo convoy daemon --drivers vfs --driver-opts vfs.path=<vfs_path>
```

#### EBS
Make sure you're running on EC2 instance, and has already [configured AWS credentials](https://github.com/aws/aws-sdk-go#configuring-credentials) correctly.
```
sudo convoy daemon --drivers ebs
```

#### DigitalOcean
Make sure you're running on a DigitalOcean Droplet, and that you have the `DO_TOKEN` environment variable set with your key
```
sudo convoy daemon --drivers digitalocean
```

## Volume Commands
#### Create a Volume

Volumes can be created using the `convoy create` command:
```
sudo convoy create volume_name
```
* Device Mapper: Default volume size is 100G. `--size` [option](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#create) is supported.
* EBS: Default volume size is 4G. `--size` and [some other options](https://github.com/rancher/convoy/blob/master/docs/ebs.md#create) are supported.

We can also create a volume using the [`docker run`](https://github.com/rancher/convoy/blob/master/docs/docker.md#create-container) command. If the volume does not yet exist, a new volume will be created. Otherwise the existing volume will be used.
```
sudo docker run -it -v test_volume:/test --volume-driver=convoy ubuntu
```

#### Delete a Volume
```
sudo convoy delete <volume_name>
```
or
```
sudo docker rm -v <container_name>
```
* NFS, EBS and DigitalOcean: The `-r/--reference` option instructs the `convoy delete` command to only delete the reference to the volume from the current host and leave the underlying files on [NFS server](https://github.com/rancher/convoy/blob/master/docs/vfs.md#delete) or [EBS volume](https://github.com/rancher/convoy/blob/master/docs/ebs.md#delete) unchanged. This is useful when the volume need to be reused later.
* [`docker rm -v`](https://github.com/rancher/convoy/blob/master/docs/docker.md#delete-container) would be treated as `convoy delete` with `-r/--reference`.
* If you use `--rm` with `docker run`, all the volumes associated with the container would be deleted in the same way as executing `docker rm -v` when exit. See [Docker run reference](https://docs.docker.com/engine/reference/run/) for details.

#### List and Inspect a Volume
```
sudo convoy list
sudo convoy inspect vol1
```

#### Take Snapshot of a Volume
```
sudo convoy snapshot create vol1 --name snap1vol1
```

#### Delete a Snapshot
```
sudo convoy snapshot delete snap1vol1
```
* Device Mapper: please make sure you keep [the latest backed-up snapshot](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#backup-create) for the same volume available to enable incremental backup mechanism, since Convoy need it to calculate the differences between snapshots.

#### Backup a Snapshot
* Device Mapper or VFS: We can backup a snapshot to S3 object store or an NFS mount/local directory:
```
sudo convoy backup create snap1vol1 --dest s3://backup-bucket@us-west-2/
```
or
```
sudo convoy backup create snap1vol1 --dest vfs:///opt/backup/
```

The backup operation returns a URL string that uniquely idenfied the backup dataset.
```
s3://backup-bucket@us-west-2/?backup=f98f9ea1-dd6e-4490-8212-6d50df1982ea\u0026volume=e0d386c5-6a24-446c-8111-1077d10356b0
```
If you're using S3, please make sure you have AWS credential ready either at ```~/.aws/credentials``` or as environment variables, as described [here](https://github.com/aws/aws-sdk-go#configuring-credentials). You may need to put credentials to ```/root/.aws/credentials``` or setup sudo environment variables in order to get S3 credential works.

* EBS: `--dest` is [not needed](https://github.com/rancher/convoy/blob/master/docs/ebs.md#backup-create). Just do `convoy backup create snap1vol1`.

#### Restore a Volume from Backup
```
sudo convoy create res1 --backup <url>
```
* EBS: Current host must be in the [same region](https://github.com/rancher/convoy/blob/master/docs/ebs.md#create) of backup to be restored.

#### Mount a Restored Volume into a Docker Container
We can use the standard `docker run` command to mount the restored volume into a Docker container:
```
sudo docker run -it -v res1:/res1 --volume-driver convoy ubuntu
```

#### Mount an NFS-Backed Volume on Multiple Servers
You can mount an NFS-backed volume on multiple servers. You can use the standard `docker run` command to mount an existing NFS-backed mount into a Docker container. For example, if you have already created an NFS-based volume `vol1` on one host, you can run the following command to mount the existing `vol1` volume into a new container:
```
sudo docker run -it -v vol1:/vol1 --volume-driver=convoy ubuntu
```
## Support and Discussion
If you need any help with Convoy, please join us at either our [forum](http://forums.rancher.com/c/convoy) or [#rancher IRC channel](http://webchat.freenode.net/?channels=rancher).

Feel free to submit any bugs, issues, and feature requests to [Convoy Issues](https://github.com/rancher/convoy/issues).

## Contribution
Contribution are welcome! Please take a look at [Development Guide](https://github.com/rancher/convoy/blob/master/docs/development.md) if you want to how to build Convoy from source or running test cases.

We love to hear new Convoy Driver ideas from you. Implemenations are most welcome! Please consider take a look at [enhancement ideas](https://github.com/rancher/convoy/labels/enhancement) if you want contribute.

And of course, [bug fixes](https://github.com/rancher/convoy/issues) are always welcome!

## References
[Convoy Command Line Reference](https://github.com/rancher/convoy/blob/master/docs/cli_reference.md)

[Using Convoy with Docker](https://github.com/rancher/convoy/blob/master/docs/docker.md)
#### Driver Specific
[Device Mapper](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md)

[Amazon Elastic Block Store](https://github.com/rancher/convoy/blob/master/docs/ebs.md)

[Virtual File System/Network File System](https://github.com/rancher/convoy/blob/master/docs/vfs.md)
