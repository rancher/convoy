# Device Mapper

## Introduction
Convoy utilizes Linux Device Mapper's thin-provisioning mechanism, to provide persistent volumes for Docker containers. The driver supports snapshot and backup/restore for the volume. Snapshotting is extremely fast and crash consistent, since Device Mapper's snapshot would only involve metadata. It also supports incremental backup, means every backup after first one would only backup the changed parts of volume, greatly reduce the storage cost and increase the speed of backup. The driver supports using S3 or VFS/NFS as backup destination.

## Daemon Options
### Driver name: ```devicemapper```
### Driver options:
#### ```dm.datadev```
__Required__. A big block device called data device used to create device mapper thin-provisioning pool. All volumes created in the future would take up the storage space in the data device. See [below](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#device-mapper-partition-helper) for how to create data device and metadata device out of single block device.
#### ```dm.metadatadev```
__Required__. A small block device called metadata device used to create device mapper thin-provisioning pool. See [below](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#device-mapper-partition-helper) for how to create data device and metadata device out of single block device, and [here](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#calculate-the-size-you-need-for-metadata-block-device) for how to calculate the necessary size of metadata device.
#### ```dm.thinpoolname```
```convoy-pool``` by default. The name of thin-provisioning pool.
#### ```dm.thinpoolblocksize```
```4096```(2MiB) by default. The block size in 512-byte sectors of thin-provisioning pool. Notice it must be a value between 128 and 2097152, and must be multiples of 128.
#### ```dm.defaultvolumesize```
```100G``` by default. Since we're using thin-provisioning volumes of device mapper, here the volume size is the upper limit of volume size, rather than real volume size allocated on the disk. Though specify a number too big here would result in bigger storage space taken by the empty filesystem.
#### ```dm.fs```
```ext4``` by default. Supported filesystem types are ext4 and xfs.

## Command details
#### `create`
* `--size` would specify the size for thin-provisioning volume. It's upper limit of volume size rather than allocated volume size on the disk.
* `--backup` accepts `s3://` and `vfs://` type of backup as long as driver used to create backup is `devicemapper`. It would create a volume with the same size of backup. If user specify a different size through `--size` option, operation would fail.

#### `inspect`
`inspect` would provides following informations at `DriverInfo` section:
* `DevID`: Device Mapper device ID.
* `Device`: Device Mapper block device location.
* `MountPoint`: Mount point of volume if mounted.
* `Size`: Volume size. It's thin-provisioning volume size, not the size allocated on the disk.

#### `info`
`info` would provides following informations at `devicemapper` section:
* `Driver`: `devicemapper`
* `Root`: Config root directory
* `DataDevice`: Data device for thin-provisioning pool
* `MetadataDevice`: Metadata device for thin-provisioning pool
* `ThinpoolDevice`: Thin-provisioning pool device
* `ThinpoolSize`: Size of thin-provisioning pool
* `ThinpoolBlockSize`: Block size of thin-provisioning pool in bytes(not in sectors as command line specified)
* `DefaultVolumeSize`: Default thin-provisioning volume size in bytes

#### `snapshot create`
`snapshot create` would use create a local Device Mapper snapshot of volume, means it's very fast, involving no data copying. The way how Device Mapper snapshot works also enable Convoy able to do incremental backup of snapshots.

#### `backup create`
`backup create` would incrementally backup a local snapshot to the backup destination. It supports `s3://` and `vfs:///` in the format of `s3://<bucket>@<region>/<path>` or `vfs:///<path>/`. Notice in order to work with S3, user need to configure AWS certificate, normally at `~/.aws/credentials`. See [here](https://github.com/aws/aws-sdk-go#configuring-credentials) for more details.

In order to make incremental backup works, the latest backed up snapshot need to be perserved. It's needed to compare with the new snapshot to find difference in order to back them up. After the new snapshot has been backed up and become the latest backed up snapshot, the old snapshot can be delete. If the latest backed up snapshot cannot be found locally, the new snapshot would be backed up in full backup way rather than in incremental backup way.

#### `snapshot inspect`:
`snapshot inspect` would provides following informations at `DriverInfo` section:
* `DevID`: Device Mapper device ID.
* `Size`: Size of thin-provisioning volume this snapshot has taken of.

#### `backup inspect`:
`backup inspect` would provides following informations:
* `BackupURL`: URL represent this backup
* `BackupUUID`: Backup's UUID
* `DriverName`: Name of Convoy Driver created this backup
* `VolumeUUID`: Original Convoy volume's UUID.
* `VolumeName`: Original Convoy volume's name.
* `VolumeSize`: Original Convoy volume's size.
* `VolumeCreatedAt`: Original Convoy volume's timestamp.
* `SnapshotUUID`: Original Convoy snapshot's UUID.
* `SnapshotName`: Original Convoy snapshot's name.
* `SnapshotCreatedAt`: Orignal Convoy snapshot's timestamp.
* `CreatedTime`: Timestamp of this backup.

## Device Mapper Partition helper
[`dm_dev_partition.sh`](https://raw.githubusercontent.com/rancher/convoy/master/tools/dm_dev_partition.sh) was created to help with setting up Device Mapper driver. It would make proper partitions out of single empty block devices automatically(see [Calculate the size you need for metadata block device](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#calculate-the-size-you-need-for-metadata-block-device)), and shows the command line to start Convoy daemon with Device Mapper driver.

Before running `dm_dev_partition.sh`, make sure Convoy has been installed correctly. Then download [`dm_dev_partition.sh`](https://raw.githubusercontent.com/rancher/convoy/master/tools/dm_dev_partition.sh). Make a dry run against the __empty__ block device you want to use first, e.g.:
```
$ sudo ./dm_dev_partition.sh /dev/xvdf
/dev/xvdf size is 107374182400 bytes
Maximum volume and snapshot counts is 10000
Metadata Device size would be 42627072 bytes
Data Device size would be 107331555328 bytes
Data Device would be 209631944 sectors
```
If there are partitions already existed on the disk, an error would be reported:
```
/dev/xvdf already partitioned, can't start partition
```
Note: the script is trying to make sure user do want to use this block device as Device Mapper's storage pool. Though user need to make sure the disk is empty and doesn't contain any valuable data.

Now we're going to create the new partitions on the device:
```
$ sudo ./dm_dev_partition.sh --write-to-disk /dev/xvdf
/dev/xvdf size is 107374182400 bytes
Maximum volume and snapshot counts is 10000
Metadata Device size would be 42627072 bytes
Data Device size would be 107331555328 bytes
Data Device would be 209631944 sectors
Start partitioning of /dev/xvdf

Complete the partition of /dev/xvdf

Disk /dev/xvdf: 107.4 GB, 107374182400 bytes
255 heads, 63 sectors/track, 13054 cylinders, total 209715200 sectors
Units = sectors of 1 * 512 = 512 bytes
Sector size (logical/physical): 512 bytes / 512 bytes
I/O size (minimum/optimal): 512 bytes / 512 bytes
Disk identifier: 0xd6e7a605

    Device Boot      Start         End      Blocks   Id  System
/dev/xvdf1            2048   209631944   104814948+  83  Linux
/dev/xvdf2       209631945   209715199       41627+  83  Linux
Now you can start Convoy Daemon using:

sudo convoy daemon --drivers devicemapper --driver-opts dm.datadev=/dev/xvdf1 --driver-opts dm.metadatadev=/dev/xvdf2
```
Now you can use the last line of output to start Convoy daemon.

## Calculate the size you need for metadata block device

Use ```convoy-pdata_tools thin_metadata_size```.

For example

1. You have a 100G block device as data device for the pool.
2. Use 2MiB block by default
3. Planning to have 100 volumes on it, each with 1000 snapshots. That is 100,000 devices in total.
```
$ convoy-pdata_tools thin_metadata_size -b 2m -s 100G -m 100000 -u M
thin_metadata_size - 411.15 megabytes estimated metadata area size for "--block-size=2mebibytes --pool-size=100gigabytes --max-thins=100000"
```
Now you see estimated size of metadata block device would be around 411MB. 

## Loopback Setup

***NOTE: This is for development purposes only. Using loopback is slow and may have other issues with data corruption***

```
truncate -s 100G data.vol
truncate -s 1G metadata.vol
sudo losetup -f data.vol
sudo losetup -f metadata.vol
```
The devices would be called ```<datadev>```(e.g. ```/dev/loop0```) and ```<metadatadev>``` (e.g. ```/dev/loop1```) respectively below.

##### Start server
```
sudo convoy daemon --drivers devicemapper --driver-opts dm.datadev=<datadev> --driver-opts dm.metadatadev=<metadatadev>
```
* Device mapper default volume size is 100G. You can override it with e.g. ```--driver-opts dm.defaultvolumesize=10G``` as stated above.
