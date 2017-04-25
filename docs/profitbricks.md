# Rancher Convoy Driver

## Table of Contents

* [Description](#description)
* [Getting Started](#getting-started)
* [Installation](#installation)
* [Usage](#usage)
    * [Authentication](#authentication)
    * [Initialize Convoy Daemon](#initialize-convoy-daemon)
* [Reference](#reference)
    * [Volumes](#volumes)
    * [Snapshots](#snapshots)
* [Examples](#examples)
    * [Create New Volume](#create-new-volume)
    * [Create New Snapshot](#create-new-snapshot)
    * [Create New Volume from Snapshot](#create-new-volume-from-snapshot)
    * [Add Existing Volume to Convoy](#add-existing-volume-to-convoy)
    * [Delete Volume Permanently](#delete-volume-permanently)
    * [Delete Volume from Convoy Only](#delete-volume-from-convoy-only)
    * [Delete Snapshot Permanently](#delete-snapshot-permanently)
    * [Docker Example](#docker-example)
* [Support](#support)
* [Testing](#testing)
* [Contributing](#contributing)

## Description
Convoy is a Docker volume plugin for a variety of storage back-ends.  Using Convoy with the ProfitBricks backend allows users to persist data across hosts and Docker containers.  ProfitBricks storage volumes can be created and destroyed using Convoy, and users can also create and restore snapshots.  The table in the [Reference](#reference) section is an exhaustive list of all operations that are currently supported by the ProfitBricks Convoy Driver.

## Getting Started
Before you begin you will need to have signed-up for a ProfitBricks account. The credentials you establish during sign-up will be used to authenticate against the ProfitBricks Cloud API.

Rancher Convoy runs on the Linux operating system.  You will need to provision a ProfitBricks server for each host that you wish to run Convoy.  Each of these hosts will need to have Docker installed and running.

## Installation
First, let's make sure we have Docker 1.8 or above running:
```
docker --version
```
If not, install the latest version of Docker as follows:
```
curl -sSL https://get.docker.com/ | sh
```
Once we've made sure we have Docker running, we can install and configure the Convoy volume plugin as follows:
```
wget https://github.com/rancher/convoy/releases/download/v0.5.0/convoy.tar.gz
tar xvzf convoy.tar.gz
sudo cp convoy/convoy convoy/convoy-pdata_tools /usr/local/bin/
sudo mkdir -p /etc/docker/plugins/
sudo bash -c 'echo "unix:///var/run/convoy/convoy.sock" > /etc/docker/plugins/convoy.spec'
```

## Usage
### Authentication
Before starting the Rancher Convoy daemon, you will need to set a few environment variables first.  These values will be used to communicate with the ProfitBricks REST API.

* `PROFITBRICKS_USERNAME` - User name associated with your ProfitBricks account.

* `PROFITBRICKS_PASSWORD` - Password for the user name specified above.

* `DATACENTER_ID` - UUID of your server's data center.

Example `~/.bash_profile`:
```
export PROFITBRICKS_USERNAME="user@example.com"
export PROFITBRICKS_PASSWORD="Password729!"
export DATACENTER_ID="a37cff2e-566b-4d8a-94e5-500fc00779de"
```
### Initialize Convoy Daemon
After you've installed Convoy and setup authentication, you are ready to start the Convoy Daemon.  The daemon runs in the foreground, and prints log messages as it performs operations.  You will therefore need to create a separate connection to the host in order to perform operations against the daemon.

Quick Start:
```
sudo convoy daemon --drivers profitbricks
```

With Driver Options:
```
sudo convoy daemon --drivers profitbricks --driver-opts profitbricks.defaultvolumesize="10G" --driver-opts profitbricks.defaultvolumetype="SSD"
```

#### Driver options:
**`profitbricks.defaultvolumesize`**

`5G` by default. ProfitBricks volumes are `1G` minimum and must be a multiple of `1G`.

**`profitbricks.defaultvolumetype`**

`HDD` by default. This can also be set to `SSD`.

## Reference

### Volumes

Below is a list of volume operations that you can perform.

#### Create
Create a new volume, create a new volume from snapshot, or add an existing volume to Convoy.

**Convoy Example**: `convoy create <name> --size "10G" --type "SSD"`<br>
**Docker Example**: `docker run -it -v <name>:/foo --volume-driver=convoy ubuntu`

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| name | no | string | Random UUID | Name of volume. |
| size | no | string | `"5G"` |  Size of volume in `GB`.  ProfitBricks volumes are `1G` minimum and must be a multiple of `1G`.  Values must be passed with the `"G"` suffix (`--size "10G"`). |
| type | no | string | `"HDD"` | Type of volume.  Choose from `"HDD"` or `"SSD"`. |
| id | no | string |  | UUID of an existing ProfitBricks volume. Convoy would use this volume instead of creating a new one. |
| backup | no | string |  | UUID of an existing ProfitBricks snapshot. Convoy would restore the snapshot into the newly created volume.  If `--size` is specified with `--backup`, then the specified size must be greater than or equal to the snapshot's size. |

* If neither `--id` nor `--backup` are specified, a new volume will be created.
* If adding an existing volume to Convoy with `--id`, you will need to manually detach the volume from its current server, if necessary.  After detaching the volume, you may then add it to Convoy, and Convoy will attach the volume to its new server.
* Creating a volume with the `docker run` command will use the default volume size and type.  To specify custom values, create the volume first using `convoy create`, and then refer to the volume by its Convoy name when executing the `docker run` command.

#### Delete
Delete a Convoy-managed volume.

**Convoy Example**: `convoy delete <name> --reference`<br>
**Docker Example**: `docker rm -v <container_name>`

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| name | no | string | | Name of volume. |
| reference | no | bool | `false` | Passing the `--reference` flag will only delete the Convoy reference to the volume, in case the user wants to preserve the volume for future use. Omitting the flag would permanently delete the volume from the ProfitBricks backend. |

* Deleting a volume with `docker rm -v <container_name>` will be treated the same as if you called `convoy delete` with the `--reference` flag.  The volume will be removed from Docker, but not from Convoy, and the volume will not be permanently deleted from the ProfitBricks backend.

#### Inspect
Returns information about a Convoy-managed volume.

**Convoy Example**: `convoy inspect <name>`

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| name | **yes** | string | | Name of volume. |

```
root@ubuntu:~# convoy inspect test_volume
{
	"Name": "test_volume",
	"Driver": "profitbricks",
	"MountPoint": "",
	"CreatedTime": "2017-03-14 05:54:25 +0000 UTC",
	"DriverInfo": {
		"AvailabilityZone": "AUTO",
		"Device": "/dev/vdb",
		"Driver": "profitbricks",
		"Id": "0cbb013f-d839-4b7f-8d33-652cf2648779",
		"MountPoint": "",
		"Size": "1073741824",
		"State": "AVAILABLE",
		"Type": "HDD",
		"VolumeCreatedAt": "2017-03-14 05:54:25 +0000 UTC",
		"VolumeName": "test_volume"
	},
	"Snapshots": {}
}
```
* `VolumeName`: Name of volume.
* `Id`: UUID of volume.
* `Size`: Size of volume, in bytes.
* `Type`: Type of volume.  (`HDD` or `SSD`.)
* `State`: Current state of volume.  (`AVAILABLE` or `BUSY`).
* `AvailablityZone`: Availability zone of volume. (`AUTO`, `ZONE_1`, `ZONE_2`, `ZONE_3`)
* `VolumeCreatedAt`: Creation time of volume.
* `Device`: Block device location of volume. (`/dev/vd_`)

#### List
Returns a map of all Convoy-managed volumes.

**Convoy Example**: `convoy list`

```
root@ubuntu:~# convoy list
{
	"another_volume": {
		"Name": "another_volume",
		"Driver": "profitbricks",
		"MountPoint": "",
		"CreatedTime": "2017-03-14 06:05:49 +0000 UTC",
		"DriverInfo": {
			"AvailabilityZone": "AUTO",
			"Device": "/dev/vdc",
			"Driver": "profitbricks",
			"Id": "cc5ac2b9-08ef-4dd4-94b3-bccaa4ace242",
			"MountPoint": "",
			"Size": "2147483648",
			"State": "AVAILABLE",
			"Type": "HDD",
			"VolumeCreatedAt": "2017-03-14 06:05:49 +0000 UTC",
			"VolumeName": "another_volume"
		},
		"Snapshots": {}
	},
	"test_volume": {
		"Name": "test_volume",
		"Driver": "profitbricks",
		"MountPoint": "",
		"CreatedTime": "2017-03-14 05:54:25 +0000 UTC",
		"DriverInfo": {
			"AvailabilityZone": "AUTO",
			"Device": "/dev/vdb",
			"Driver": "profitbricks",
			"Id": "0cbb013f-d839-4b7f-8d33-652cf2648779",
			"MountPoint": "",
			"Size": "1073741824",
			"State": "AVAILABLE",
			"Type": "HDD",
			"VolumeCreatedAt": "2017-03-14 05:54:25 +0000 UTC",
			"VolumeName": "test_volume"
		},
		"Snapshots": {}
	}
}
```

### Snapshots

Below is a list of snapshot operations that you can perform.

#### Create
Create a new snapshot.

**Convoy Example**: `convoy snapshot create <volume_name> --name "test_snapshot"`<br>

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| volume_name | **yes** | string | | Snapshot will be created from this volume. |
| name | no | string | Random UUID |  Name of snapshot. |

#### Delete
Delete a Convoy-managed snapshot.

**Convoy Example**: `convoy snapshot delete <name>`<br>

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| name | **yes** | string | |  Name of snapshot. |
* You will need to manually delete the snapshot from the ProfitBricks backend.

#### Inspect
Returns information about a Convoy-managed snapshot.

**Convoy Example**: `convoy snapshot inspect <name>`

| Name | Required | Type | Default | Description |
| --- | :-: | --- | --- | --- |
| name | **yes** | string | | Name of snapshot. |

```
root@ubuntu:~# convoy snapshot inspect test_snapshot
{
	"Name": "test_snapshot",
	"VolumeName": "test_volume",
	"VolumeCreatedAt": "2017-03-14 05:54:25 +0000 UTC",
	"CreatedTime": "2017-03-14 07:30:38 +0000 UTC",
	"DriverInfo": {
		"Description": "Created from \"test_volume\" in Data Center \"Convoy\"",
		"Driver": "profitbricks",
		"Id": "dd3378dc-c1dc-4140-be47-27fea3eb5bb1",
		"Location": "us/las",
		"Size": "1073741824",
		"SnapshotCreatedAt": "2017-03-14 07:30:38 +0000 UTC",
		"SnapshotName": "test_snapshot",
		"State": "AVAILABLE"
	}
}
```
* `SnapshotName`: Name of the snapshot.
* `Id`: UUID of the snapshot.
* `Description`: Description of the snapshot.
* `Size`: Size of the snapshot, in bytes.
* `State`: Current state of the snapshot.  (`AVAILABLE` or `BUSY`).
* `Location`: Data center location of the snapshot. (`us/las`, `de/fkb`, `de/fra`)
* `SnapshotCreatedAt`: Creation time of snapshot.

## Examples

#### Create New Volume
```
convoy create blank_volume --size "8G" --type "SSD"
```

#### Create New Snapshot
```
convoy snapshot create blank_volume --name "snapshot1"
```

#### Create New Volume from Snapshot
```
convoy create from_snapshot --backup "3f8128b6-4de7-424d-baef-b606dc4aae9a"
```

#### Add Existing Volume to Convoy
```
convoy create existing_volume --id "0cbb013f-d839-4b7f-8d33-652cf2648779"
```

#### Delete Volume Permanently
```
convoy delete blank_volume
```

#### Delete Volume from Convoy Only
```
convoy delete blank_volume --reference
```

#### Delete Snapshot Permanently
```
convoy snapshot delete snapshot1
```

#### Docker Example
In this example, we implicitly create a volume named `volume1` with the `docker run` command.  Docker will spin up a container, and we will use `touch` to create a file at `/volume1/foo`.  We then exit the first container.

Spin up a new container using the same volume, `volume1`, and list the contents of `/volume1/foo` to ensure that our data has been preserved from one container to the next.

Creating a volume with the `docker run` command will use the default volume size and type.  To use custom values instead, first you must create the volume using `convoy create`, and then refer to the volume in Docker by its Convoy name.
```
root@ubuntu:~# docker run -it -v volume1:/volume1 --volume-driver=convoy ubuntu
root@c7d73b8733e9:/# touch /volume1/foo
root@c7d73b8733e9:/# exit
exit
root@ubuntu:~# docker run -it -v volume1:/volume1 --volume-driver=convoy ubuntu
root@4b3a4ef0d35a:/# ls /volume1/foo
/volume1/foo
root@4b3a4ef0d35a:/# exit
exit
```

## Support
You are welcome to contact us with questions or comments using the Community section of the [ProfitBricks DevOps Central](https://devops.profitbricks.com/). Please report any feature requests or issues using GitHub issue tracker.

* [Rancher Convoy](https://github.com/rancher/convoy) repository on GitHub.
* Ask a question or discuss at [ProfitBricks DevOps Central](https://devops.profitbricks.com/community/).
* Report an [issue here](https://github.com/rancher/convoy/issues).

## Testing
In order to run unit tests for the ProfitBricks Driver, you will need to ensure that
a working Golang environment is setup on your Linux machine.  Instructions for setting
up a Golang environment can be found [here](https://golang.org/doc/install).

**Warning:** Running the test suite will provision resources on your ProfitBricks account.

1. Set [environment variables](#authentication) so that the test suite can authenticate against the ProfitBricks REST API.
2. Download the source code `go get "github.com/rancher/convoy"`.
3. Navigate to `$GO_PATH/src/github.com/rancher/convoy/profitbricks/`.
4. Run `go test`.
5. If successful, test output should look something like:
```
root@ubuntu:~/go_projects/src/github.com/rancher/convoy/profitbricks# go test
DEBU[0000] Creating blank volume
DEBU[0007] Creating snapshot
DEBU[0043] Creating volume from snapshot
DEBU[0075] Adding pre-existing volume to Convoy
DEBU[0078] Deleting snapshot
DEBU[0079] Deleting complex volumes
DEBU[0080] Creating blank volume
DEBU[0101] Attaching volume
DEBU[0117] Checking device path
DEBU[0117] Testing GET volume
DEBU[0117] Creating snapshot
DEBU[0153] Testing GET snapshot
DEBU[0154] Deleting snapshot
DEBU[0154] Deleting volume
OK: 3 passed
PASS
ok  	github.com/rancher/convoy/profitbricks	154.839s
```

## Contributing
1. Fork the repository. (https://github.com/rancher/convoy/fork)
2. Create your feature branch. (`git checkout -b my-new-feature`)
3. Commit your changes. (`git commit -am 'Add some feature'`)
4. Push to the branch. (`git push origin my-new-feature`)
5. Create a new Pull Request.
