package api

type VolumeMountRequest struct {
	VolumeUUID string
	MountPoint string
	Verbose    bool
}

type VolumeUmountRequest struct {
	VolumeUUID string
}

type VolumeCreateRequest struct {
	Name       string
	DriverName string
	Size       int64
	BackupURL  string
	Verbose    bool
}

type VolumeDeleteRequest struct {
	VolumeUUID    string
	ReferenceOnly bool
}

type VolumeInspectRequest struct {
	VolumeUUID string
}

type SnapshotCreateRequest struct {
	Name       string
	VolumeUUID string
	Verbose    bool
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
	Verbose      bool
}

type BackupDeleteRequest struct {
	URL string
}
