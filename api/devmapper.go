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
	Volumes []DeviceMapperVolume
}

type DeviceMapperVolume struct {
	UUID      string
	DevID     int
	Size      int64
	Snapshots []DeviceMapperSnapshot
}

type DeviceMapperSnapshot struct {
	UUID  string
	DevID int
}
