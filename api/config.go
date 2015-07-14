package api

type VolumeMountConfig struct {
	MountPoint string
	NameSpace  string
}

type ObjectStoreRegisterConfig struct {
	Kind string
	Opts map[string]string
}

type VolumeListConfig struct {
	DriverSpecific bool
}

type VolumeCreateConfig struct {
	Name string
	Size int64
}
