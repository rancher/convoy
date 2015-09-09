# Device Mapper

## Initialization
### Driver name: ```devicemapper```
### Driver options:
#### ```dm.datadev```
Data device used to create device mapper thin-provisioning pool.
#### ```dm.metadatadev```
Metadata device used to create device mapper thin-provisioning pool.
#### ```dm.thinpoolname```
```convoy-pool``` by default. The name of thin-provisioning pool.
#### ```dm.thinpoolblocksize```
```4096```(2MiB) by default. The block size in 512-byte sectors of thin-provisioning pool. Notice it must be a value between 128 and 2097152, and must be multiples of 128.
#### ```dm.defaultvolumesize```
```100G``` by default. Since we're using thin-provisioning volumes of device mapper, here the volume size is the upper limit of volume size, rather than real volume size allocated on the disk. Though specify a number too big here would result in bigger storage space taken by the empty filesystem.

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
sudo convoy server --drivers devicemapper --driver-opts dm.datadev=<datadev> --driver-opts dm.metadatadev=<metadatadev>
```
* Device mapper default volume size is 100G. You can override it with e.g. ```--driver-opts dm.defaultvolumesize=10G```
