package api

type VolumeMountConfig struct {
	MountPoint string
	FileSystem string
	Options    string
	NeedFormat bool
	NameSpace  string
}

type BlockStoreRegisterConfig struct {
	Kind string
	Opts map[string]string
}

type BlockStoreImageConfig struct {
	ImageFile string
}
