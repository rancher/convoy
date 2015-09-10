# Virtual File System / Network File System
## Introduction

VFS/NFS driver would create a directory for each volume at user specified location(`vfs.path`), and store all the content of volume in that directory. The driver can be used either locally, or remotely by mounting NFS to `vfs.path`. If `vfs.path` is mounted NFS path, then the volume can be shared across the servers by using the same NFS mount and refer to the volume name on the other servers.

VFS/NFS driver implements snapshot/backup as an compressed single file, supports using S3 or VFS/NFS as backup destination.

## Daemon Options
### Driver Name: `vfs`
### Driver options:
#### `vfs.path`
The directory used to store volumes. Can be local directory or mounted NFS directory.

## Command details
#### `create`
* `create` would create a directory named `volume_name` at `vfs.path`, and use that directory to store volume.
  * E.g., `vfs.path` is set to `/opt/nfs-volumes/`. Then user creates a new volume named `vol1`, then a directory named `/opt/nfs-volumes/vol1` would be created and volume contents would be stored in it.
* If the directory named `volume_name` already existed, it would be used instead of creating a new directory for volume
  * E.g., `vfs.path` is set to `/opt/nfs-volumes/`, and `/opt/nfs-volumes/vol1` already exists. When user creates a new volume named `vol1`, the directory `/opt/nfs-volumes/vol1` would be picked up automatically as the directroy for volume, keeping all the existing files intact.
* `--backup` accepts `s3://` and `vfs://` as long as the driver used to create the backup is `vfs`.

#### `delete`
`delete` would delete the directory where the volume stored by default.
* `--reference` would only delete the reference of volume in Convoy. It would perserve the volume directory for future use.
  * E.g., `vfs.path` is set to `/opt/nfs-volumes/`, and user has created volume `vol1`. `convoy delete --reference vol1` would result in remove the reference of `vol1` in Convoy, but keep the directory `/opt/nfs-volumes/vol1` for future use.

#### `inspect`
`inspect` would provides following informations at `DriverInfo` section:
* `Path`: Directory where the volume stored.
* `MountPoint`: Mount point of the volume if mounted.

#### `info`
`info` would provides following informations at `vfs` section:
* `Root`: VFS config root directory
* `Path`: Directory used to store volumes.

#### `snapshot create`
`snapshot create` would create a compressed tarball of volume directory.

#### `snapshot inspect`
`snapshot inspect` would provides following informations at `DriverInfo` section:
* `FilePath`: The compressed tarball location of snapshot.

#### `backup create`
`backup create` would copy the compressed tarball to the destination location.

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
