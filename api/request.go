package api

type VolumeMountRequest struct {
	VolumeUUID string
	MountPoint string
}

type VolumeUmountRequest struct {
	VolumeUUID string
}

type VolumeCreateRequest struct {
	Name      string
	Size      int64
	BackupURL string
}

type VolumeDeleteRequest struct {
	VolumeUUID string
}

type VolumeInspectRequest struct {
	VolumeUUID string
}

type SnapshotCreateRequest struct {
	Name       string
	VolumeUUID string
}

type SnapshotDeleteRequest struct {
	SnapshotUUID string
}

type SnapshotInspectRequest struct {
	SnapshotUUID string
}

type BackupListRequest struct {
	URL          string
	VolumeUUID   string
	SnapshotUUID string
}

type BackupCreateRequest struct {
	URL          string
	SnapshotUUID string
}

type BackupDeleteRequest struct {
	URL string
}
