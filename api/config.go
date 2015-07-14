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

type ObjectStoreListConfig struct {
	URL          string
	VolumeUUID   string
	SnapshotUUID string
}

type ObjectStoreBackupConfig struct {
	URL          string
	SnapshotUUID string
}

type ObjectStoreRestoreConfig struct {
	URL                string
	SourceVolumeUUID   string
	SourceSnapshotUUID string
	TargetVolumeUUID   string
}

type ObjectStoreDeleteConfig struct {
	URL          string
	VolumeUUID   string
	SnapshotUUID string
}
