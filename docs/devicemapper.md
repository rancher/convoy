# Device Mapper

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
