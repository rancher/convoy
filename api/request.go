package api

type VolumeMountRequest struct {
	VolumeName string
	MountPoint string
	ReadWrite  string // either "rw or "ro"
	BindMount  string // either "bind" or "rbind"
	ReMount    bool   // allow or disallow mount with a new mountpoint
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
	FSType         string
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
