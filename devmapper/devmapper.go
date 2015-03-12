// +build linux
package devmapper

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
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
)

type Driver struct {
	configFile string
	Device
}

type Volume struct {
	DevId     int
	Size      uint64
	Snapshots map[string]Snapshot
}

type Snapshot struct {
	DevId int
}

type Device struct {
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      uint64
	ThinpoolBlockSize uint32
	LastDevId         int
	Volumes           map[string]Volume
}

func init() {
	drivers.Register(DRIVER_NAME, Init)
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
	dv.ThinpoolDevice = DM_DIR + config[DM_THINPOOL_NAME]

	if _, exists := config[DM_THINPOOL_BLOCK_SIZE]; !exists {
		config[DM_THINPOOL_BLOCK_SIZE] = DEFAULT_BLOCK_SIZE
	}

	blockSizeString := config[DM_THINPOOL_BLOCK_SIZE]
	blockSizeTmp, err := strconv.Atoi(blockSizeString)
	if err != nil {
		return nil, fmt.Errorf("Illegal block size specified")
	}
	blockSize := uint32(blockSizeTmp)

	if blockSize < BLOCK_SIZE_MIN || blockSize > BLOCK_SIZE_MAX || blockSize%BLOCK_SIZE_MULTIPLIER != 0 {
		return nil, fmt.Errorf("Block size must between %v and %v, and must be a multiple of %v",
			BLOCK_SIZE_MIN, BLOCK_SIZE_MAX, BLOCK_SIZE_MULTIPLIER)
	}

	dv.ThinpoolBlockSize = blockSize

	return &dv, nil
}

func Init(root string, config map[string]string) (drivers.Driver, error) {
	driverConfig := filepath.Join(root, DRIVER_NAME) + ".cfg"
	if _, err := os.Stat(driverConfig); err == nil {
		dev := Device{
			Volumes: make(map[string]Volume),
		}
		err := utils.LoadConfig(driverConfig, &dev)
		d := &Driver{}
		if err != nil {
			return d, err
		}
		d.Device = dev
		d.configFile = driverConfig
		return d, nil
	}

	dev, err := verifyConfig(config)
	if err != nil {
		return nil, err
	}

	dev.Root = root
	dev.Volumes = make(map[string]Volume)

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
	dev.ThinpoolSize = thinpSize
	dev.LastDevId = 1

	err = createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, dev.ThinpoolBlockSize)
	if err != nil {
		return nil, err
	}

	err = utils.SaveConfig(driverConfig, &dev)
	if err != nil {
		return nil, err
	}
	d := &Driver{
		configFile: driverConfig,
		Device:     *dev,
	}
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

func (d *Driver) CreateVolume(id, baseId string, size uint64) error {
	if size%uint64(d.ThinpoolBlockSize*SECTOR_SIZE) != 0 {
		return fmt.Errorf("Size must be multiple of block size")

	}
	devId := d.LastDevId
	log.Debugf("Creating device, uuid %v(devid %v)", id, devId)
	err := devicemapper.CreateDevice(d.ThinpoolDevice, devId)
	if err != nil {
		return err
	}
	log.Debug("Created device")

	log.Debugf("Activating device, uuid %v(devid %v)", id, devId)
	err = devicemapper.ActivateDevice(d.ThinpoolDevice, id, devId, size)
	if err != nil {
		devicemapper.DeleteDevice(d.ThinpoolDevice, devId)
		log.Debugf("Removed device due to fail to activate, uuid %v devid %v", id, devId)
		return err
	}
	log.Debug("Activated device")

	volume := Volume{
		DevId:     devId,
		Size:      size,
		Snapshots: make(map[string]Snapshot),
	}
	d.Volumes[id] = volume
	d.LastDevId++

	if err = d.updateConfig(); err != nil {
		return err
	}

	return nil
}

func (d *Driver) DeleteVolume(id string) error {
	volume, exists := d.Volumes[id]
	if !exists {
		return fmt.Errorf("Cannot find volume with uuid %v", id)
	}
	if len(volume.Snapshots) != 0 {
		return fmt.Errorf("Volume %v still contains snapshots, delete snapshots first", id)
	}

	log.Debugf("Deactivating device, uuid %v(devid %v)", id, volume.DevId)
	err := devicemapper.RemoveDevice(id)
	if err != nil {
		return err
	}
	log.Debug("Deactivatd device")
	log.Debugf("Deleting device, uuid %v(devid %v)", id, volume.DevId)
	err = devicemapper.DeleteDevice(d.ThinpoolDevice, volume.DevId)
	if err != nil {
		return err
	}
	log.Debug("Deleted device")

	delete(d.Volumes, id)

	if err = d.updateConfig(); err != nil {
		return err
	}
	return nil
}

func (d *Driver) ListVolumes() error {
	for uuid, volume := range d.Volumes {
		fmt.Printf("volume %v\n", uuid)
		fmt.Println("\tdev id:", volume.DevId)
		fmt.Println("\tsize:", volume.Size)
	}
	return nil
}

func (d *Driver) updateConfig() error {
	return utils.SaveConfig(d.configFile, d.Device)
}

func (d *Driver) CreateSnapshot(id, volumeId string) error {
	volume, exists := d.Volumes[volumeId]
	if !exists {
		return fmt.Errorf("Cannot find volume with uuid %v", volumeId)
	}
	devId := d.LastDevId

	log.Debugf("Creating snapshot %v for volume %v", id, volumeId)
	err := devicemapper.CreateSnapDevice(d.ThinpoolDevice, devId, volumeId, volume.DevId)
	if err != nil {
		return err
	}
	log.Debugf("Created snapshot")

	snapshot := Snapshot{
		DevId: devId,
	}
	volume.Snapshots[id] = snapshot
	d.LastDevId++

	if err = d.updateConfig(); err != nil {
		return err
	}
	return nil
}

func (d *Driver) DeleteSnapshot(id, volumeId string) error {
	volume, exists := d.Volumes[volumeId]
	if !exists {
		return fmt.Errorf("Cannot find volume with uuid %v", volumeId)
	}

	snapshot, exists := volume.Snapshots[id]
	if !exists {
		return fmt.Errorf("Cannot find snapshot with uuid %v in volume %v", id, volumeId)
	}

	log.Debugf("Deleting snapshot %v for volume %v", id, volumeId)
	err := devicemapper.DeleteDevice(d.ThinpoolDevice, snapshot.DevId)
	if err != nil {
		return err
	}
	log.Debug("Deleted snapshot")
	delete(volume.Snapshots, id)

	if err = d.updateConfig(); err != nil {
		return err
	}
	return nil
}

func listVolumeSnapshot(volumeId string, volume Volume) {
	fmt.Printf("Volume %v\n", volumeId)
	for uuid, snapshot := range volume.Snapshots {
		fmt.Printf("snapshot %v\n", uuid)
		fmt.Println("\tdev id:", snapshot.DevId)
	}
}

func (d *Driver) ListSnapshot(volumeId string) error {
	if volumeId == "" {
		for uuid, volume := range d.Volumes {
			listVolumeSnapshot(uuid, volume)
		}
	}
	volume := d.Volumes[volumeId]
	listVolumeSnapshot(volumeId, volume)
	return nil
}

func (d *Driver) ExportSnapshot(id, path string, blockSize uint32) error {
	return nil
}

func (d *Driver) Info() error {
	// from sector count to byte
	blockSize := d.ThinpoolBlockSize * 512

	fmt.Println("\tworking directory:", d.Root)
	fmt.Println("\tdata device:", d.DataDevice)
	fmt.Println("\tmetadata device:", d.MetadataDevice)
	fmt.Println("\tthinpool:", d.ThinpoolDevice)
	fmt.Println("\tthinpool size:", d.ThinpoolSize)
	fmt.Println("\tthinpool block size:", blockSize)

	return nil
}
