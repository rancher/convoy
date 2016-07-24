# GlusterFS
## Introduction

GlusterFS is a popular distributed network filesystem. Convoy can leveage GlusterFS to create volumes for Docker container.

## Daemon Options
### Driver Name: `glusterfs`
### Driver options:
#### `glusterfs.servers`
__Required__. The server list of GlusterFS. Can be host name or IP address. Separate by "," without space. e.g. `10.1.1.2,10.1.1.3,10.1.1.4`
#### `glusterfs.defaultvolumepool`
__Required__. The default GlusterFS volume name which would be used to create container volumes. The GlusterFS volume would be used to create multiple container volumes.

## Command details
#### `create`
* `create` would create a directory named `volume_name` at mounted path of default GlusterFS volume, and use that directory to store volume.
  * E.g., the default GlusterFS volume is mounted to `/var/lib/convoy/glusterfs/mounts/my_vol`. Then user creates a new volume named `vol1`, then a directory named `/var/lib/convoy/glusterfs/mounts/my_vol` would be created and volume contents would be stored in it.
* If the directory named `volume_name` already existed, it would be used instead of creating a new directory for volume
  * E.g., the default GlusterFS volume is mounted to `/var/lib/convoy/glusterfs/mounts/my_vol`, and `/var/lib/convoy/glusterfs/mounts/my_vol/vol1` already exists. When user creates a new volume named `vol1`, the directory `/var/lib/convoy/glusterfs/mounts/my_vol/vol1` would be picked up automatically as the directroy for volume, keeping all the existing files intact.

#### `delete`
`delete` would delete the directory where the volume stored by default.
* `--reference` would only delete the reference of volume in Convoy. It would preserve the volume directory for future use.
  * E.g., the default GlusterFS volume is mounted to `/var/lib/convoy/glusterfs/mounts/my_vol`, and user has created volume `vol1`. `convoy delete --reference vol1` would result in remove the reference of `vol1` in Convoy, but keep the directory `/var/lib/convoy/glusterfs/mounts/my_vol/vol1` for future use.

#### `inspect`
`inspect` would provides following informations at `DriverInfo` section:
* `Name`: The volume name.
* `Path`: Directory where the volume stored.
* `MountPoint`: Mount point of the volume if mounted.
* `GlusterFSVolume`: The name of GlusterFS volume used to store this container volume.
* `GlusterFSServers`: The servers for GlusterFS volume.

#### `info`
`info` would provides following informations at `vfs` section:
* `Root`: Convoy's GlusterFS config root directory.
* `GlusterFSServers`: The servers for GlusterFS volume.
* `DefaultVolumePool`: The default GlusterFS volume name which would be used to create container volumes.

#### Snapshot and Backup are not supported at this stage
