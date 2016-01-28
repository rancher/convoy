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
	Name           string
	DriverName     string
	Size           int64
	BackupURL      string
	DriverVolumeID string
	Type           string
	IOPS           int64
	PrepareForVM   bool
	Verbose        bool
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
	SnapshotName string
}

type SnapshotInspectRequest struct {
	SnapshotName string
}

type BackupListRequest struct {
	URL          string
	VolumeUUID   string
	SnapshotName string
}

type BackupCreateRequest struct {
	URL          string
	SnapshotName string
	Verbose      bool
}

type BackupDeleteRequest struct {
	URL string
}
