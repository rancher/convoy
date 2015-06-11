package api

type DeviceMapperInfo struct {
	Driver            string
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      int64
	ThinpoolBlockSize int64
}

type DeviceMapperVolumes struct {
	Volumes map[string]DeviceMapperVolume
}

type DeviceMapperVolume struct {
	DevID     int
	Snapshots map[string]DeviceMapperSnapshot
}

type DeviceMapperSnapshot struct {
	DevID int
}
