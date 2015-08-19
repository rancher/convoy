# Device Mapper Loopback Setup

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
