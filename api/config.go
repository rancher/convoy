package api

type VolumeMountConfig struct {
	MountPoint string
	FileSystem string
	Options    string
	NeedFormat bool
	NameSpace  string
}

type ObjectStoreRegisterConfig struct {
	Kind string
	Opts map[string]string
}

type ObjectStoreImageConfig struct {
	ImageFile string
}

type VolumeListConfig struct {
	DriverSpecific bool
}
