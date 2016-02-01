package api

type VolumeMountRequest struct {
	VolumeName string
	MountPoint string
	Verbose    bool
}

type VolumeUmountRequest struct {
	VolumeName string
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
	VolumeName    string
	ReferenceOnly bool
}

type VolumeInspectRequest struct {
	VolumeName string
}

type SnapshotCreateRequest struct {
	Name       string
	VolumeName string
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
	VolumeName   string
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
