package api

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
