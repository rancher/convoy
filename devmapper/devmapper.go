// +build linux

package devmapper

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/metadata"
	"github.com/rancherio/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	DRIVER_NAME           = "devicemapper"
	DEFAULT_THINPOOL_NAME = "rancher-volume-pool"
	DEFAULT_BLOCK_SIZE    = "4096"
	DM_DIR                = "/dev/mapper/"

	DM_DATA_DEV            = "dm.datadev"
	DM_METADATA_DEV        = "dm.metadatadev"
	DM_THINPOOL_NAME       = "dm.thinpoolname"
	DM_THINPOOL_BLOCK_SIZE = "dm.thinpoolblocksize"

	// as defined in device mapper thin provisioning
	BLOCK_SIZE_MIN        = 128
	BLOCK_SIZE_MAX        = 2097152
	BLOCK_SIZE_MULTIPLIER = 128

	SECTOR_SIZE = 512

	VOLUME_CFG_PREFIX  = "volume_"
	VOLUME_CFG_POSTFIX = "_" + DRIVER_NAME + ".json"
)

type Driver struct {
	root       string
	configName string
	Device
}

type Volume struct {
	UUID      string
	DevID     int
	Size      int64
	Snapshots map[string]Snapshot
}

type Snapshot struct {
	DevID     int
	Activated bool
}

type Device struct {
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      int64
	ThinpoolBlockSize int64
	LastDevID         int
}

func init() {
	drivers.Register(DRIVER_NAME, Init)
}

func getVolumeCfgName(uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("Invalid volume UUID specified: %v", uuid)
	}
	return VOLUME_CFG_PREFIX + uuid + VOLUME_CFG_POSTFIX, nil
}

func (device *Device) loadVolume(uuid string) *Volume {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return nil
	}
	if !utils.ConfigExists(device.Root, cfgName) {
		return nil
	}
	volume := &Volume{}
	if err := utils.LoadConfig(device.Root, cfgName, volume); err != nil {
		log.Error("Failed to load volume json ", cfgName)
		return nil
	}
	return volume
}

func (device *Device) saveVolume(volume *Volume) error {
	uuid := volume.UUID
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return utils.SaveConfig(device.Root, cfgName, volume)
}

func (device *Device) deleteVolume(uuid string) error {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return utils.RemoveConfig(device.Root, cfgName)
}

func (device *Device) listVolumeIDs() []string {
	return utils.ListConfigIDs(device.Root, VOLUME_CFG_PREFIX, VOLUME_CFG_POSTFIX)
}

func verifyConfig(config map[string]string) (*Device, error) {
	dv := Device{
		DataDevice:     config[DM_DATA_DEV],
		MetadataDevice: config[DM_METADATA_DEV],
	}

	if dv.DataDevice == "" || dv.MetadataDevice == "" {
		return nil, fmt.Errorf("data device or metadata device unspecified")
	}

	if _, exists := config[DM_THINPOOL_NAME]; !exists {
		config[DM_THINPOOL_NAME] = DEFAULT_THINPOOL_NAME
	}
	dv.ThinpoolDevice = filepath.Join(DM_DIR, config[DM_THINPOOL_NAME])

	if _, exists := config[DM_THINPOOL_BLOCK_SIZE]; !exists {
		config[DM_THINPOOL_BLOCK_SIZE] = DEFAULT_BLOCK_SIZE
	}

	blockSizeString := config[DM_THINPOOL_BLOCK_SIZE]
	blockSize, err := strconv.ParseInt(blockSizeString, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Illegal block size specified")
	}
	if blockSize < BLOCK_SIZE_MIN || blockSize > BLOCK_SIZE_MAX || blockSize%BLOCK_SIZE_MULTIPLIER != 0 {
		return nil, fmt.Errorf("Block size must between %v and %v, and must be a multiple of %v",
			BLOCK_SIZE_MIN, BLOCK_SIZE_MAX, BLOCK_SIZE_MULTIPLIER)
	}

	dv.ThinpoolBlockSize = blockSize

	return &dv, nil
}

func (d *Driver) activatePool() error {
	dev := d.Device
	if _, err := os.Stat(dev.ThinpoolDevice); err == nil {
		log.Debug("Found created pool, skip pool reinit")
		return nil
	}

	dataDev, err := os.Open(dev.DataDevice)
	if err != nil {
		return err
	}
	defer dataDev.Close()

	metadataDev, err := os.Open(dev.MetadataDevice)
	if err != nil {
		return err
	}
	defer metadataDev.Close()

	if err := createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, uint32(dev.ThinpoolBlockSize)); err != nil {
		return err
	}
	log.Debug("Reinitialized the existing pool ", dev.ThinpoolDevice)

	volumeIDs := dev.listVolumeIDs()
	for _, id := range volumeIDs {
		volume := dev.loadVolume(id)
		if err := d.activateDevice(id, volume.DevID, uint64(volume.Size)); err != nil {
			return err
		}
		log.Debug("Reactivated volume device", id)
	}
	return nil
}

func Init(root, cfgName string, config map[string]string) (drivers.Driver, error) {
	if utils.ConfigExists(root, cfgName) {
		dev := Device{}
		err := utils.LoadConfig(root, cfgName, &dev)
		d := &Driver{}
		if err != nil {
			return d, err
		}
		d.Device = dev
		d.configName = cfgName
		d.root = root
		if err := d.activatePool(); err != nil {
			return d, err
		}
		return d, nil
	}

	dev, err := verifyConfig(config)
	if err != nil {
		return nil, err
	}

	dev.Root = root

	dataDev, err := os.Open(dev.DataDevice)
	if err != nil {
		return nil, err
	}
	defer dataDev.Close()

	metadataDev, err := os.Open(dev.MetadataDevice)
	if err != nil {
		return nil, err
	}
	defer metadataDev.Close()

	thinpSize, err := devicemapper.GetBlockDeviceSize(dataDev)
	if err != nil {
		return nil, err
	}
	dev.ThinpoolSize = int64(thinpSize)
	dev.LastDevID = 1

	err = createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, uint32(dev.ThinpoolBlockSize))
	if err != nil {
		return nil, err
	}

	err = utils.SaveConfig(root, cfgName, &dev)
	if err != nil {
		return nil, err
	}
	d := &Driver{
		root:       root,
		configName: cfgName,
		Device:     *dev,
	}
	log.Debug("Init done")
	return d, nil
}

func createPool(poolName string, dataDev, metadataDev *os.File, blockSize uint32) error {
	err := devicemapper.CreatePool(poolName, dataDev, metadataDev, blockSize)
	if err != nil {
		return err
	}
	log.Debugln("Created pool /dev/mapper/" + poolName)

	return nil
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) CreateVolume(id, baseID string, size int64) error {
	if size%(d.ThinpoolBlockSize*SECTOR_SIZE) != 0 {
		return fmt.Errorf("Size must be multiple of block size")

	}
	volume := d.loadVolume(id)
	if volume != nil {
		return fmt.Errorf("Already has volume with uuid %v", id)
	}

	devID := d.LastDevID
	log.Debugf("Creating device, uuid %v(devid %v)", id, devID)
	err := devicemapper.CreateDevice(d.ThinpoolDevice, devID)
	if err != nil {
		return err
	}
	log.Debug("Created device")

	if err = d.activateDevice(id, devID, uint64(size)); err != nil {
		devicemapper.DeleteDevice(d.ThinpoolDevice, devID)
		log.Debugf("Removed device due to fail to activate, uuid %v devid %v", id, devID)
		return err
	}

	volume = &Volume{
		UUID:      id,
		DevID:     devID,
		Size:      size,
		Snapshots: make(map[string]Snapshot),
	}
	if err := d.saveVolume(volume); err != nil {
		return err
	}
	d.LastDevID++

	if err := utils.SaveConfig(d.root, d.configName, d.Device); err != nil {
		return err
	}
	log.Debug("Created volume ", id)

	return nil
}

func (d *Driver) DeleteVolume(id string) error {
	var err error
	volume := d.loadVolume(id)
	if volume == nil {
		return fmt.Errorf("cannot find volume %v", id)
	}
	if len(volume.Snapshots) != 0 {
		return fmt.Errorf("Volume %v still contains snapshots, delete snapshots first", id)
	}

	if err = d.deactivateDevice(id, volume.DevID); err != nil {
		return err
	}

	log.Debugf("Deleting device, uuid %v(devid %v)", id, volume.DevID)
	err = devicemapper.DeleteDevice(d.ThinpoolDevice, volume.DevID)
	if err != nil {
		return err
	}
	log.Debug("Deleted device")

	if err := d.deleteVolume(id); err != nil {
		return err
	}

	log.Debug("Deleted volume ", id)
	return nil
}

func getVolumeSnapshotInfo(uuid string, volume *Volume, snapshotID string) *api.DeviceMapperVolume {
	result := api.DeviceMapperVolume{
		DevID:     volume.DevID,
		Size:      volume.Size,
		Snapshots: make(map[string]api.DeviceMapperSnapshot),
	}
	if s, exists := volume.Snapshots[snapshotID]; exists {
		result.Snapshots[snapshotID] = api.DeviceMapperSnapshot{
			DevID: s.DevID,
		}
	}
	return &result
}

func getVolumeInfo(uuid string, volume *Volume) *api.DeviceMapperVolume {
	result := api.DeviceMapperVolume{
		DevID:     volume.DevID,
		Size:      volume.Size,
		Snapshots: make(map[string]api.DeviceMapperSnapshot),
	}
	for uuid, snapshot := range volume.Snapshots {
		s := api.DeviceMapperSnapshot{
			DevID: snapshot.DevID,
		}
		result.Snapshots[uuid] = s
	}
	return &result
}

func (d *Driver) ListVolume(id, snapshotID string) error {
	volumes := api.DeviceMapperVolumes{
		Volumes: make(map[string]api.DeviceMapperVolume),
	}
	if id != "" {
		volume := d.loadVolume(id)
		if volume == nil {
			return fmt.Errorf("volume %v doesn't exists", id)
		}
		if snapshotID != "" {
			volumes.Volumes[id] = *getVolumeSnapshotInfo(id, volume, snapshotID)
		} else {
			volumes.Volumes[id] = *getVolumeInfo(id, volume)
		}

	} else {
		volumeIDs := d.listVolumeIDs()
		for _, uuid := range volumeIDs {
			volume := d.loadVolume(uuid)
			if volume == nil {
				return fmt.Errorf("Volume list changed, for volume %v", uuid)
			}
			volumes.Volumes[uuid] = *getVolumeInfo(uuid, volume)
		}
	}
	api.ResponseOutput(volumes)
	return nil
}

func (d *Driver) CreateSnapshot(id, volumeID string) error {
	volume := d.loadVolume(volumeID)
	if volume == nil {
		return fmt.Errorf("Cannot find volume with uuid %v", volumeID)
	}
	devID := d.LastDevID

	snapshot, exists := volume.Snapshots[id]
	if exists {
		return fmt.Errorf("Already has snapshot with uuid %v", id)
	}

	log.Debugf("Creating snapshot %v for volume %v", id, volumeID)
	err := devicemapper.CreateSnapDevice(d.ThinpoolDevice, devID, volumeID, volume.DevID)
	if err != nil {
		return err
	}
	log.Debugf("Created snapshot device")

	snapshot = Snapshot{
		DevID:     devID,
		Activated: false,
	}
	volume.Snapshots[id] = snapshot
	d.LastDevID++

	if err := d.saveVolume(volume); err != nil {
		return err
	}
	if err := utils.SaveConfig(d.root, d.configName, d.Device); err != nil {
		return err
	}
	log.Debug("Created snapshot", id)
	return nil
}

func (d *Driver) DeleteSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.Debugf("Deleting snapshot %v for volume %v", id, volumeID)
	err = devicemapper.DeleteDevice(d.ThinpoolDevice, snapshot.DevID)
	if err != nil {
		return err
	}
	log.Debug("Deleted snapshot device")
	delete(volume.Snapshots, id)

	if err = d.saveVolume(volume); err != nil {
		return err
	}
	log.Debug("Deleted snapshot", id)
	return nil
}

func (d *Driver) CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error) {
	includeSame := false
	if compareID == "" || compareID == id {
		compareID = id
		includeSame = true
	}
	snap1, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}
	snap2, _, err := d.getSnapshotAndVolume(compareID, volumeID)
	if err != nil {
		return nil, err
	}

	dev := d.MetadataDevice
	out, err := exec.Command("pdata_tools", "thin_delta",
		"--snap1", strconv.Itoa(snap1.DevID), "--snap2", strconv.Itoa(snap2.DevID), dev).Output()
	if err != nil {
		return nil, err
	}
	mapping, err := metadata.DeviceMapperThinDeltaParser(out, d.ThinpoolBlockSize*SECTOR_SIZE, includeSame)
	if err != nil {
		return nil, err
	}
	return mapping, err
}

func (d *Driver) Info() error {
	// from sector count to byte
	blockSize := d.ThinpoolBlockSize * 512

	dmInfo := api.DeviceMapperInfo{
		Driver:            d.Name(),
		Root:              d.Root,
		DataDevice:        d.DataDevice,
		MetadataDevice:    d.MetadataDevice,
		ThinpoolDevice:    d.ThinpoolDevice,
		ThinpoolSize:      d.ThinpoolSize,
		ThinpoolBlockSize: blockSize,
	}

	api.ResponseOutput(dmInfo)

	return nil
}

func (d *Driver) getSnapshotAndVolume(snapshotID, volumeID string) (*Snapshot, *Volume, error) {
	volume := d.loadVolume(volumeID)
	if volume == nil {
		return nil, nil, fmt.Errorf("cannot find volume %v", volumeID)
	}
	snap, exists := volume.Snapshots[snapshotID]
	if !exists {
		return nil, nil, fmt.Errorf("cannot find snapshot %v of volume %v", snapshotID, volumeID)
	}
	return &snap, volume, nil
}

func (d *Driver) activateDevice(id string, devID int, size uint64) error {
	log.Debugf("Activating device, uuid %v(devid %v)", id, devID)
	err := devicemapper.ActivateDevice(d.ThinpoolDevice, id, devID, size)
	if err != nil {
		return err
	}
	log.Debug("Activated device")
	return nil
}

func (d *Driver) deactivateDevice(id string, devID int) error {
	log.Debugf("Deactivating device, uuid %v(devid %v)", id, devID)
	err := devicemapper.RemoveDevice(id)
	if err != nil {
		return err
	}
	log.Debug("Deactivated device")
	return nil
}

func (d *Driver) OpenSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	if err = d.activateDevice(id, snapshot.DevID, uint64(volume.Size)); err != nil {
		return err
	}
	snapshot.Activated = true

	return d.saveVolume(volume)
}

func (d *Driver) CloseSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	if err := d.deactivateDevice(id, snapshot.DevID); err != nil {
		return err
	}
	snapshot.Activated = false

	return d.saveVolume(volume)
}

func (d *Driver) ReadSnapshot(id, volumeID string, offset int64, data []byte) error {
	_, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	dev := filepath.Join(DM_DIR, id)
	devFile, err := os.Open(dev)
	if err != nil {
		return err
	}
	defer devFile.Close()

	if _, err = devFile.ReadAt(data, offset); err != nil {
		return err
	}

	return nil
}

func (d *Driver) GetVolumeDevice(id string) (string, error) {
	volume := d.loadVolume(id)
	if volume == nil {
		return "", fmt.Errorf("Volume with uuid %v doesn't exist", id)
	}

	return filepath.Join(DM_DIR, id), nil
}

func (d *Driver) HasSnapshot(id, volumeID string) bool {
	_, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return false
	}
	return true
}
