package api

type VolumeMountConfig struct {
	MountPoint string
	NameSpace  string
}

type VolumeListConfig struct {
	DriverSpecific bool
}

type VolumeCreateConfig struct {
	Name string
	Size int64
}

type BackupListConfig struct {
	URL          string
	VolumeUUID   string
	SnapshotUUID string
}

type BackupCreateConfig struct {
	URL          string
	SnapshotUUID string
}

type BackupRestoreConfig struct {
	URL              string
	TargetVolumeUUID string
}

type BackupDeleteConfig struct {
	URL string
}
