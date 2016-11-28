# Convoy Command Line Reference

## Top level commands

```
COMMANDS:
   daemon	start convoy daemon
   info		information about convoy
   create	create a new volume: create [volume_name] [options]
   delete	delete a volume: delete <volume> [options]
   mount	mount a volume to an specific path: mount <volume> [options]
   umount	umount a volume: umount <volume> [options]
   list		list all managed volumes
   inspect	inspect a certain volume: inspect <volume>
   snapshot	snapshot related operations
   backup	backup related operations
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --socket, -s "/var/run/convoy/convoy.sock"	Specify unix domain socket for communication between server and client
   --debug, -d					Enable debug level log with client or not
   --verbose					Verbose level output for client, for create volume/snapshot etc
   --help, -h					show help
   --version, -v				print the version
```

#### daemon
```
NAME:
   daemon - start convoy daemon

USAGE:
   command daemon [command options] [arguments...]

OPTIONS:
   --debug							Debug log, enabled by default
   --log 							specific output log file, otherwise output to stdout by default
   --root "/var/lib/convoy"					specific root directory of convoy, if configure file exists, daemon specific options would be ignored
   --config                         Config filename for driver
```
1. ```daemon``` command would start the Convoy daemon.The same Convoy binary would be used to start daemon as well as used as the client to communicate with daemon. In order to use Convoy, user need to setup and start the Convoy daemon first. Convoy daemon would run in the foreground by default. User can use various method e.g. [init-script](https://github.com/fhd/init-script-template) to start Convoy as background daemon.
2. ```--root``` option would specify Convoy daemon's config root directory. After start Convoy on the host for the first time, it would contains all the information necessary for Convoy to start. After first time of start up, ```convoy daemon``` would automatically load configuration from config root directory. User don't need to specify same configurations anymore.
3. ```--drivers``` and ```--driver-opts``` can be specified multiple times. ```--drivers``` would be the name of Convoy Driver, and ````--driver-opts``` would be the options for initialize the certain driver. See [```devicemapper```](https://github.com/rancher/convoy/blob/master/docs/devicemapper.md#driver-initialization), ```vfs```, ```ebs``` for driver option details. If there are multiple drivers specified, the first one in the list would be the default driver. See ```convoy create``` for details.


#### info
```
NAME:
   info - information about convoy

USAGE:
   command info [arguments...]
```

#### create
```
NAME:
   create - create a new volume: create [volume_name] [options]

USAGE:
   command create [command options] [arguments...]

OPTIONS:
   --storagetype        specify using storagetype
   --size 	size of volume if driver supports, in bytes, or end in either G or M or K
   --backup 	create a volume of backup if driver supports
   --id 	driver specific volume ID if driver supports
   --type 	driver specific volume type if driver supports
   --iops 	IOPS if driver supports
```
1. ```create``` command would create a volume. ```volume_name``` is optional. If no ```volume_name``` specified, an automatically name would be generated in format of ```volume-xxxxxxxx```, in which last 8 characters would be the first 8 characters of volume's automatical generated UUID. The ```volume_name``` here would be the name user used with Docker.
2. ```--driver``` option would be used to specify which driver to use if there are more than one driver supported in the setup. Without the option, the default driver(first driver in the list of ```--drivers``` when executing ```daemon``` command) would be used.
3. ```--size``` option would be used to specify a volume's size if driver supports. Current it's supported by ```devicemapper``` and ```ebs```.
4. ```--backup``` option would be used to specify create a volume from existing backup. The backup would be in a format of URL and can be driver specific. See [backup] command for more details.
5. ```--id```, ```--type```, ```--iops``` are driver specific options. Currenty they're supported by ```ebs```.

#### delete
```
NAME:
   delete - delete a volume: delete <volume> [options]

USAGE:
   command delete [command options] [arguments...]

OPTIONS:
   --reference, -r	only delete the reference of volume if driver supports
```
1. Volume can be referred by name, UUID, or partial UUID.
2. ```--reference``` would only delete the reference of volume if driver supports. It provides ability to retain the volume after volume no longer managed by Convoy. Current it's supported by ```vfs``` and ```ebs```. 

#### mount
```
NAME:
   mount - mount a volume: mount <volume> [options]

USAGE:
   command mount [command options] [arguments...]

OPTIONS:
   --mountpoint 	mountpoint of volume, if not specified, it would be automatic mounted to default directory
```
* Volume can be referred by name, UUID, or partial UUID.

#### umount
```
NAME:
   umount - umount a volume: umount <volume> [options]

USAGE:
   command umount [arguments...]
```
* Volume can be referred by name, UUID, or partial UUID.

#### list
```
NAME:
   list - list all managed volumes

USAGE:
   command list [command options] [arguments...]

OPTIONS:
   --driver	Ask for driver specific info of volumes and snapshots
```

#### inspect
```
NAME:
   inspect - inspect a certain volume: inspect <volume>

USAGE:
   command inspect [arguments...]

OPTIONS:
   --help, -h   show help
```
* Volume can be referred by name, UUID, or partial UUID.

## snapshot
```
NAME:
   convoy snapshot - snapshot related operations

USAGE:
   convoy snapshot command [command options] [arguments...]

COMMANDS:
   create	create a snapshot for certain volume: snapshot create <volume>
   delete	delete a snapshot: snapshot delete <snapshot>
   inspect	inspect an snapshot: snapshot inspect <snapshot>
   help, h	Shows a list of commands or help for one command

OPTIONS:
   --help, -h	show help
```
* For using this subcommand with ```ebs```, see ```ebs``` for details.

#### create
```
NAME:
   snapshot create - create a snapshot for certain volume: snapshot create <volume>

USAGE:
   command snapshot create [command options] [arguments...]

OPTIONS:
   --name 	name of snapshot
```
* Volume can be referred by name, UUID, or partial UUID.

#### delete
```
NAME:
   snapshot delete - delete a snapshot: snapshot delete <snapshot>

USAGE:
   command snapshot delete [arguments...]
```
* Snapshot can be referred by name, UUID, or partial UUID.

#### inspect
```
NAME:
   snapshot inspect - inspect an snapshot: snapshot inspect <snapshot>

USAGE:
   command snapshot inspect [arguments...]
```
* Snapshot can be referred by name, UUID, or partial UUID.

## backup
```
NAME:
   convoy backup - backup related operations

USAGE:
   convoy backup command [command options] [arguments...]

COMMANDS:
   create	create a backup in objectstore: create <snapshot>
   delete	delete a backup in objectstore: delete <backup>
   list		list volume in objectstore: list <dest>
   inspect	inspect a backup: inspect <backup>
   help, h	Shows a list of commands or help for one command

OPTIONS:
   --help, -h	show help
```
* For using this subcommand with ```ebs```, see ```ebs``` for details.

#### create
```
NAME:
   backup create - create a backup in objectstore: create <snapshot>

USAGE:
   command backup create [command options] [arguments...]

OPTIONS:
   --dest 	destination of backup if driver supports, would be url like s3://bucket@region/path/ or vfs:///path/
```
1. Snapshot can be referred by name, UUID, or partial UUID.
2. This command would create a backup from existing snapshot, making it possible to restore this backup to a volume in the future. The command would return a backup represented by a URL for future references.
3. There are two kinds of backup destination(objectstores as we called them) supported today, ```s3``` and ```vfs```. For using AWS S3 as backup destination, user need to setup S3 certificate first, see [here](http://blogs.aws.amazon.com/security/post/Tx3D6U6WSFGOK2H/A-New-and-Standardized-Way-to-Manage-Credentials-in-the-AWS-SDKs) for more information. And ```vfs``` destination can be a mounted NFS.

#### delete
```
NAME:
   backup delete - delete a backup in objectstore: delete <backup>

USAGE:
   command backup delete [arguments...]
```

#### list
```
NAME:
   backup list - list backups in objectstore: list <dest>

USAGE:
   command backup list [command options] [arguments...]

OPTIONS:
   --volume-uuid 	uuid of volume
```
1. It's likely a costly operation, since it would list all the possible backups in the objectstore. So it's better to filter it with ```--volume-uuid```
2. The command is not supported by ```ebs```. See ```ebs``` for details.

#### inspect
```
NAME:
   backup inspect - inspect a backup: inspect <backup>

USAGE:
   command backup inspect [arguments...]
```
