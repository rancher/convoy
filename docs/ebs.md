# Amazon Elastic Block Store

## Introduction
If user is running Convoy on AWS EC2 instance, Convoy would be able to create EBS volumes attached directly to the Docker container. It suits for mission critial or performance critical tasks.

Convoy would create a EBS volume for user, attach it to the current running instance, and assign it to the Docker container. Convoy can also take snapshot of the volume and back it up, then create a new volume from the backup. The snapshot and backup operations are implemented using EBS snapshot mechanism. Further more, Convoy can take an existing EBS volume and use it for Docker container as well.

Notice user would be billed for EBS volume and snapshots from Amazon.

## AWS IAM permission
User need to set correct permission for using Convoy with AWS, refer to [AWS](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-policies-for-amazon-ec2.html) for more details.

At least please add permission for these actions:

```
"ec2:CreateSnapshot",
"ec2:CreateTags",
"ec2:CreateVolume",
"ec2:DeleteVolume",
"ec2:AttachVolume",
"ec2:DetachVolume",
"ec2:DescribeSnapshots",
"ec2:DescribeTags",
"ec2:DescribeVolumes"
```

## Daemon Options

### Driver name: `ebs`
### Driver options:
#### `ebs.defaultvolumesize`
`4G` by default. EBS volumes are 1GiB minimal and must be a multiple of 1GiB.
#### `ebs.defaultvolumetype`
`gp2` by default. See [Amazon EBS Volume Types](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html) for details. Notice if user choose `io1` as default volume type, then user has to specify `--iops` when creating volume everytime.
Other values are st1 and sc1.
#### `ebs.defaultkmskeyid`
Default is blank, if specified than volumes will be encrypted using the given kms key id.
#### `ebs.defaultencrypted`
`false` by default, if `true` then volumes will be encrypted with the default account kms key.
#### `ebs.fsfreeze`
Default is false.  If set to true, will perform a `/sbin/fsfreeze` on the filesystem before creating a snapshot, and unfreeze after the snapshot has been created.  This may yield a more consistent snapshot of a running application.  This uses `/sbin/fsfreeze` command which must be installed.  It is installed by default in Ubuntu 16.04 based docker images.
## Command details
### `create`
* `--size` would specify the EBS volume size user want to create. EBS volumes are 1GiB minimal and must be a multiple of 1GiB.
* `--id` would specify an existing EBS volume ID in order to reuse it. Convoy would use this volume instead of creating a new one.
* `--type` would specify an [Amazon EBS Volume Types](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSVolumeTypes.html) for the volume to be created. Notice if `io1` is used, `--iops` option would be required as well.
* `--iops` is required and only valid when `--type io1` is specified. See [EBS I/O Characteristics](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ebs-io-characteristics.html) for details.
* `--backup` accepts `ebs://` type of backup only. It would create a new volume with [EBS snapshot](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/EBSSnapshots.html) specified by the backup. If `--size` is specified with `--backup`, specified size must equal or bigger than original EBS snapshot. Also the EBS snapshot represented by the backup must be in the same region of current instance, since copying snapshot from different region would take too long and stagnates volume creation process.
* If neither `--id` nor `--backup` specified, a new volume would be created as options specified and formatted to `ext4` filesystem.
* The maximum volume attached to one EC2 instance is limited. Due to the limitation of Linux device names, Amazon suggested limit the number of volumes to 11(`/dev/sd[f-p]`), when volumes are attached to EC2 HVM instance. See [Device Naming on Linux Instances](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html) for more info.

### `delete`
* By default `delete` would delete the underlaying EBS volume.
* `--reference` would only delete the reference of underlaying EBS volume in Convoy, in case user want to preserve the volume for future use.

### `inspect`
`inspect` would provides following informations at `DriverInfo` section:
* `Device`: EBS block device location
* `MountPoint`: Mount point of volume is mounted
* `EBSVolumeID`: EBS volume ID of the volume.
* `AvailiablityZone`: Availability Zone of the volume. It has to be the same as instance.
* `CreatedTime`: Timestamp of EBS volume.
* `Size`: EBS volume size, in bytes.
* `State`: EBS volume state. Should be `InUse` when it's attached to the current instance.
* `Type`: EBS volume type.
* `IOPS`: Input/Output Operations Per Second for EBS volume.
* `KmsKeyId`: If the volume is encrypted, this specifies be the KMS key used.

### `snapshot create`
`snapshot create` would create a new EBS snapshot of current EBS volume. The command would return immediately after it confirmed that creating of an EBS snapshot has been initated.

### `snapshot delete`
`snapshot delete` would remove the reference of the EBS snapshot in Convoy. The command won't delete the EBS snapshot. Deletion of EBS snapshot would be done by `backup delete`.

### `snapshot inspect`
`snapshot inspect` would provides following informations at `DriverInfo` section:
* `EBSSnapshotID`: EBS snapshot ID
* `EBSVolumeID`: Original EBS volume ID
* `StartTime`: Timestamp of start creating EBS snapshot
* `Size`: Size of original EBS volume.
* `State`: EBS snapshot state. Would be either `completed`, `error` or `pending`
* `KmsKeyId`: If the snapshot is encrypted, this specifies the KMS key used.

### `backup create`
`backup create` would wait for a EBS snapshot complete creating if it hasn't completed yet. If creation of snapshot was success, the command would return URL in the format of `ebs://<region>/snap-xxxxxxxx` represent the backup, which can be used with `create --backup` command later.

`--dest` option is not supported with EBS driver.

### `backup delete`
`backup delete` would take `ebs://<region>/snap-xxxxxxxx` and delete `snap-xxxxxxxx` in AWS `region`.

### `backup inspect`
`backup inspect` would return following informations:
* `EBSSnapshotID`: EBS snapshot ID
* `EBSVolumeID`: Original EBS volume ID
* `StartTime`: Timestamp of start creating EBS snapshot
* `Size`: Size of original EBS volume.
* `State`: EBS snapshot state. Would be either `completed`, `error` or `pending`
* `KmsKeyId`: If the snapshot is encrypted, this specifies the KMS key used.

## AWS tags
Convoy uses the following bookeeping tags on EBS volume/snapshots which can be used to classify convoy managed resources.

### EBS Volume
* `Name`: Volume Name In Convoy
* `ConvoyVolumeUUID`: Volume UUID In Convoy

### EBS Snapshot
* `ConvoyVolumeUUID`: Related Volume UUID In Convoy
* `ConvoySnapshotUUID`: Snapshot UUID in Convoy
