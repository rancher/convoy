# Device Mapper

## Driver initialization
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
```100G``` by default. Since we're using thin-provisioning volumes of device mapper, here the volume size is the upper limit of volume size, rather than real volume size occupied the disk. Though specify a number too big here would result in bigger storage space taken by the empty filesystem.

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
