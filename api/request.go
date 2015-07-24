package api

type VolumeMountRequest struct {
	MountPoint string
}

type VolumeCreateRequest struct {
	Name      string
	Size      int64
	BackupURL string
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
