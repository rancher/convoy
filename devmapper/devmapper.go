// +build linux

package devmapper

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/metadata"
	"github.com/rancher/rancher-volume/storagedriver"
	"github.com/rancher/rancher-volume/util"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	. "github.com/rancher/rancher-volume/logging"
)

const (
	DRIVER_NAME           = "devicemapper"
	DRIVER_CONFIG_FILE    = "devicemapper.cfg"
	DEFAULT_THINPOOL_NAME = "rancher-volume-pool"
	DEFAULT_BLOCK_SIZE    = "4096"
	DM_DIR                = "/dev/mapper/"
	MOUNTS_DIR            = "mounts"

	THIN_PROVISION_TOOLS_BINARY      = "pdata_tools"
	THIN_PROVISION_TOOLS_MIN_VERSION = "0.5"

	DM_DATA_DEV            = "dm.datadev"
	DM_METADATA_DEV        = "dm.metadatadev"
	DM_THINPOOL_NAME       = "dm.thinpoolname"
	DM_THINPOOL_BLOCK_SIZE = "dm.thinpoolblocksize"

	// as defined in device mapper thin provisioning
	BLOCK_SIZE_MIN        = 128
	BLOCK_SIZE_MAX        = 2097152
	BLOCK_SIZE_MULTIPLIER = 128

	SECTOR_SIZE = 512

	VOLUME_CFG_PREFIX    = "volume_"
	IMAGE_CFG_PREFIX     = "image_"
	DEVMAPPER_CFG_PREFIX = DRIVER_NAME + "_"
	CFG_POSTFIX          = ".json"

	DM_LOG_FIELD_VOLUME_DEVID   = "dm_volume_devid"
	DM_LOG_FIELD_SNAPSHOT_DEVID = "dm_snapshot_devid"

	MOUNT_BINARY  = "mount"
	UMOUNT_BINARY = "umount"

	DMLogLevel = devicemapper.LogLevelDebug
)

type Driver struct {
	configName string
	Mutex      *sync.Mutex
	Device
}

type Volume struct {
	UUID       string
	DevID      int
	Size       int64
	Base       string
	MountPoint string
	Snapshots  map[string]Snapshot
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

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "devmapper"})
)

type DMLogger struct{}

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("devmapper", fields, format, v)
}

func init() {
	storagedriver.Register(DRIVER_NAME, Init)
}

func getVolumeCfgName(uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("Invalid volume UUID specified: %v", uuid)
	}
	return DEVMAPPER_CFG_PREFIX + VOLUME_CFG_PREFIX + uuid + CFG_POSTFIX, nil
}

func (device *Device) loadVolume(uuid string) *Volume {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return nil
	}
	if !util.ConfigExists(device.Root, cfgName) {
		return nil
	}
	volume := &Volume{}
	if err := util.LoadConfig(device.Root, cfgName, volume); err != nil {
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
	return util.SaveConfig(device.Root, cfgName, volume)
}

func (device *Device) deleteVolume(uuid string) error {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return util.RemoveConfig(device.Root, cfgName)
}

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, DEVMAPPER_CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
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

	volumeIDs, err := dev.listVolumeIDs()
	if err != nil {
		return err
	}
	for _, id := range volumeIDs {
		volume, err := d.checkLoadVolume(id)
		if err != nil {
			return err
		}
		if err := devicemapper.ActivateDevice(dev.ThinpoolDevice, id, volume.DevID, uint64(volume.Size)); err != nil {
			return err
		}
		log.WithFields(logrus.Fields{
			LOG_FIELD_EVENT:  LOG_EVENT_ACTIVATE,
			LOG_FIELD_VOLUME: id,
		}).Debug("Reactivated volume device")
	}
	return nil
}

func (logger *DMLogger) DMLog(level int, file string, line int, dmError int, message string) {
	// By default libdm sends us all the messages including debug ones.
	// We need to filter out messages here and figure out which one
	// should be printed.
	if level > DMLogLevel {
		return
	}

	if level <= devicemapper.LogLevelErr {
		logrus.Errorf("libdevmapper(%d): %s:%d (%d) %s", level, file, line, dmError, message)
	} else if level <= devicemapper.LogLevelInfo {
		logrus.Infof("libdevmapper(%d): %s:%d (%d) %s", level, file, line, dmError, message)
	} else {
		logrus.Debugf("libdevmapper(%d): %s:%d (%d) %s", level, file, line, dmError, message)
	}
}

func (d *Driver) remountVolumes() error {
	volumeIDs, err := d.listVolumeIDs()
	if err != nil {
		return err
	}
	for _, uuid := range volumeIDs {
		volume, err := d.checkLoadVolume(uuid)
		if err != nil {
			return err
		}
		if volume.MountPoint == "" {
			continue
		}
		if _, err := d.MountVolume(uuid, map[string]string{}); err != nil {
			return err
		}
	}
	return err
}

func Init(root string, config map[string]string) (storagedriver.StorageDriver, error) {
	devicemapper.LogInitVerbose(1)
	devicemapper.LogInit(&DMLogger{})

	if err := checkEnvironment(); err != nil {
		return nil, err
	}

	if err := util.MkdirIfNotExists(root); err != nil {
		return nil, err
	}
	cfgName := DRIVER_CONFIG_FILE
	if util.ConfigExists(root, cfgName) {
		dev := Device{}
		err := util.LoadConfig(root, cfgName, &dev)
		d := &Driver{
			Mutex: &sync.Mutex{},
		}
		if err != nil {
			return d, err
		}
		d.Device = dev
		d.configName = cfgName
		if err := d.activatePool(); err != nil {
			return d, err
		}
		if err := d.remountVolumes(); err != nil {
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
	dev.LastDevID = 0

	err = createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, uint32(dev.ThinpoolBlockSize))
	if err != nil {
		return nil, err
	}

	err = util.SaveConfig(root, cfgName, &dev)
	if err != nil {
		return nil, err
	}
	d := &Driver{
		configName: cfgName,
		Device:     *dev,
		Mutex:      &sync.Mutex{},
	}
	log.Debug("Init done")
	return d, nil
}

func (d *Driver) VolumeOps() (storagedriver.VolumeOperations, error) {
	return d, nil
}

func (d *Driver) SnapshotOps() (storagedriver.SnapshotOperations, error) {
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

func (d *Driver) allocateDevID() (int, error) {
	d.Mutex.Lock()
	defer d.Mutex.Unlock()

	d.LastDevID++
	log.Debug("Current devID ", d.LastDevID)
	if err := util.SaveConfig(d.Root, d.configName, d.Device); err != nil {
		return 0, err
	}
	return d.LastDevID, nil
}

func (d *Driver) CreateVolume(id string, size int64) error {
	var err error
	if size%(d.ThinpoolBlockSize*SECTOR_SIZE) != 0 {
		return fmt.Errorf("Size must be multiple of block size")

	}
	volume := d.loadVolume(id)
	if volume != nil {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME: id,
		}, "Already has volume with specific uuid")
	}

	devID, err := d.allocateDevID()
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:          LOG_REASON_START,
		LOG_FIELD_EVENT:           LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:          LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:          id,
		DM_LOG_FIELD_VOLUME_DEVID: devID,
	}).Debugf("Creating volume")
	err = devicemapper.CreateDevice(d.ThinpoolDevice, devID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:          LOG_REASON_START,
		LOG_FIELD_EVENT:           LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:          LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:          id,
		DM_LOG_FIELD_VOLUME_DEVID: devID,
	}).Debugf("Activating device for volume")
	err = devicemapper.ActivateDevice(d.ThinpoolDevice, id, devID, uint64(size))
	if err != nil {
		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON:          LOG_REASON_ROLLBACK,
			LOG_FIELD_EVENT:           LOG_EVENT_REMOVE,
			LOG_FIELD_OBJECT:          LOG_OBJECT_VOLUME,
			LOG_FIELD_VOLUME:          id,
			DM_LOG_FIELD_VOLUME_DEVID: devID,
		}).Debugf("Removing device for volume due to fail to activate")
		if err := devicemapper.DeleteDevice(d.ThinpoolDevice, devID); err != nil {
			log.WithFields(logrus.Fields{
				LOG_FIELD_REASON:          LOG_REASON_FAILURE,
				LOG_FIELD_EVENT:           LOG_EVENT_REMOVE,
				LOG_FIELD_OBJECT:          LOG_OBJECT_VOLUME,
				LOG_FIELD_VOLUME:          id,
				DM_LOG_FIELD_VOLUME_DEVID: devID,
			}).Debugf("Failed to remove device")
		}
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

	// format the device
	dev, err := d.GetVolumeDevice(id)
	if err != nil {
		return err
	}
	if _, err := util.Execute("mkfs", []string{"-t", "ext4", dev}); err != nil {
		return err
	}

	return nil
}

func (d *Driver) DeleteVolume(id string) error {
	var err error
	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return err
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %s, it hasn't been umounted", id)
	}
	if len(volume.Snapshots) != 0 {
		for snapshotUUID := range volume.Snapshots {
			if err = d.DeleteSnapshot(snapshotUUID, volume.UUID); err != nil {
				return generateError(logrus.Fields{
					LOG_FIELD_VOLUME:   volume.UUID,
					LOG_FIELD_SNAPSHOT: snapshotUUID,
				}, "cannot remove an snapshot of volume, as part of deletion of volume")
			}
		}
	}

	if err = devicemapper.RemoveDevice(id); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:          LOG_REASON_START,
		LOG_FIELD_EVENT:           LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:          LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:          id,
		DM_LOG_FIELD_VOLUME_DEVID: volume.DevID,
	}).Debugf("Deleting device")
	err = devicemapper.DeleteDevice(d.ThinpoolDevice, volume.DevID)
	if err != nil {
		return err
	}

	if err := d.deleteVolume(id); err != nil {
		return err
	}
	return nil
}

func getVolumeSnapshotInfo(uuid string, volume *Volume, snapshotID string) *api.DeviceMapperVolume {
	result := api.DeviceMapperVolume{
		DevID:     volume.DevID,
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

func (d *Driver) ListVolume(id string) ([]byte, error) {
	volumes := api.DeviceMapperVolumes{
		Volumes: make(map[string]api.DeviceMapperVolume),
	}
	if id != "" {
		volume, err := d.checkLoadVolume(id)
		if err != nil {
			return nil, err
		}
		volumes.Volumes[id] = *getVolumeInfo(id, volume)
	} else {
		volumeIDs, err := d.listVolumeIDs()
		if err != nil {
			return nil, err
		}
		for _, uuid := range volumeIDs {
			volume, err := d.checkLoadVolume(uuid)
			if err != nil {
				return nil, err
			}
			volumes.Volumes[uuid] = *getVolumeInfo(uuid, volume)
		}
	}
	return api.ResponseOutput(volumes)
}

func (d *Driver) CreateSnapshot(id, volumeID string) error {
	var err error

	volume, err := d.checkLoadVolume(volumeID)
	if err != nil {
		return err
	}
	devID, err := d.allocateDevID()
	if err != nil {
		return err
	}

	snapshot, exists := volume.Snapshots[id]
	if exists {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:   volumeID,
			LOG_FIELD_SNAPSHOT: id,
		}, "Already has snapshot with uuid")
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:            LOG_REASON_START,
		LOG_FIELD_EVENT:             LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:            LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:          id,
		LOG_FIELD_VOLUME:            volumeID,
		DM_LOG_FIELD_VOLUME_DEVID:   volume.DevID,
		DM_LOG_FIELD_SNAPSHOT_DEVID: devID,
	}).Debugf("Creating snapshot")
	err = devicemapper.CreateSnapDevice(d.ThinpoolDevice, devID, volumeID, volume.DevID)
	if err != nil {
		return err
	}
	log.Debugf("Created snapshot device")

	snapshot = Snapshot{
		DevID:     devID,
		Activated: false,
	}
	volume.Snapshots[id] = snapshot

	if err := d.saveVolume(volume); err != nil {
		return err
	}
	return nil
}

func (d *Driver) DeleteSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: id,
		LOG_FIELD_VOLUME:   volumeID,
	}).Debugf("Deleting snapshot for volume")
	err = devicemapper.DeleteDevice(d.ThinpoolDevice, snapshot.DevID)
	if err != nil {
		return err
	}
	log.Debug("Deleted snapshot device")
	delete(volume.Snapshots, id)

	if err = d.saveVolume(volume); err != nil {
		return err
	}
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
	out, err := util.Execute(THIN_PROVISION_TOOLS_BINARY, []string{"thin_delta",
		"--snap1", strconv.Itoa(snap1.DevID),
		"--snap2", strconv.Itoa(snap2.DevID),
		dev})
	if err != nil {
		return nil, err
	}
	mapping, err := metadata.DeviceMapperThinDeltaParser([]byte(out), d.ThinpoolBlockSize*SECTOR_SIZE, includeSame)
	if err != nil {
		return nil, err
	}
	return mapping, err
}

func (d *Driver) Info() ([]byte, error) {
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

	data, err := api.ResponseOutput(dmInfo)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (d *Driver) checkLoadVolume(volumeID string) (*Volume, error) {
	volume := d.loadVolume(volumeID)
	if volume == nil {
		return nil, generateError(logrus.Fields{
			LOG_FIELD_VOLUME: volumeID,
		}, "Cannot find volume")
	}
	return volume, nil
}

func (d *Driver) getSnapshotAndVolume(snapshotID, volumeID string) (*Snapshot, *Volume, error) {
	volume := d.loadVolume(volumeID)
	if volume == nil {
		return nil, nil, generateError(logrus.Fields{
			LOG_FIELD_VOLUME: volumeID,
		}, "cannot find volume")
	}
	snap, exists := volume.Snapshots[snapshotID]
	if !exists {
		return nil, nil, generateError(logrus.Fields{
			LOG_FIELD_VOLUME:   volumeID,
			LOG_FIELD_SNAPSHOT: snapshotID,
		}, "cannot find snapshot of volume")
	}
	return &snap, volume, nil
}

func (d *Driver) OpenSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:            LOG_REASON_START,
		LOG_FIELD_EVENT:             LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:            LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_VOLUME:            volumeID,
		LOG_FIELD_SNAPSHOT:          id,
		LOG_FIELD_SIZE:              volume.Size,
		DM_LOG_FIELD_SNAPSHOT_DEVID: snapshot.DevID,
	}).Debug()
	if err = devicemapper.ActivateDevice(d.ThinpoolDevice, id, snapshot.DevID, uint64(volume.Size)); err != nil {
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

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: id,
	}).Debug()
	if err := devicemapper.RemoveDevice(id); err != nil {
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
		return "", generateError(logrus.Fields{
			LOG_FIELD_VOLUME: id,
		}, "Volume doesn't exist")
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

func removePool(poolName string) error {
	err := devicemapper.RemoveDevice(poolName)
	if err != nil {
		return err
	}
	log.Debugln("Removed pool /dev/mapper/" + poolName)

	return nil
}

func (d *Driver) deactivatePool() error {
	dev := d.Device

	volumeIDs, err := dev.listVolumeIDs()
	if err != nil {
		return err
	}
	for _, id := range volumeIDs {
		volume := dev.loadVolume(id)
		if volume == nil {
			return generateError(logrus.Fields{
				LOG_FIELD_VOLUME: id,
			}, "Cannot find volume")
		}
		if err := devicemapper.RemoveDevice(id); err != nil {
			return err
		}
		log.WithFields(logrus.Fields{
			LOG_FIELD_EVENT:  LOG_EVENT_ACTIVATE,
			LOG_FIELD_VOLUME: id,
		}).Debug("Deactivated volume device")
	}

	if err := removePool(dev.ThinpoolDevice); err != nil {
		return err
	}
	log.Debug("Deactivate the pool ", dev.ThinpoolDevice)
	return nil
}

func checkEnvironment() error {
	/* disable udev sync support for static link binary
	if supported := devicemapper.UdevSetSyncSupport(true); !supported {
		return nil, fmt.Errorf("Udev sync is not supported. Cannot proceed.")
	} */
	cmdline := []string{"thin_delta", "-V"}
	if err := util.CheckBinaryVersion(THIN_PROVISION_TOOLS_BINARY, THIN_PROVISION_TOOLS_MIN_VERSION, cmdline); err != nil {
		return err
	}
	return nil
}

func mounted(dev, mountPoint string) bool {
	output, err := util.Execute("mount", []string{})
	if err != nil {
		return false
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, dev) && strings.Contains(line, mountPoint) {
			return true
		}
	}
	return false
}

func (d *Driver) getVolumeMountPoint(volumeUUID, specifiedPoint string) (string, error) {
	var dir string
	if specifiedPoint != "" {
		dir = specifiedPoint
	} else {
		dir = filepath.Join(d.Root, MOUNTS_DIR, volumeUUID)
	}
	if err := util.MkdirIfNotExists(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func (d *Driver) MountVolume(id string, opts map[string]string) (string, error) {
	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return "", err
	}
	dev, err := d.GetVolumeDevice(id)
	if err != nil {
		return "", err
	}
	specifiedPoint := opts[storagedriver.OPTS_MOUNT_POINT]
	mountPoint, err := d.getVolumeMountPoint(id, specifiedPoint)
	if err != nil {
		return "", err
	}
	if volume.MountPoint != "" && volume.MountPoint != mountPoint {
		return "", fmt.Errorf("volume %v already mounted at %v, but asked to mount at %v", id, volume.MountPoint, mountPoint)
	}
	if !mounted(dev, mountPoint) {
		log.Debugf("Volume %v is not mounted, mount it now to %v", id, mountPoint)
		_, err = util.Execute(MOUNT_BINARY, []string{dev, mountPoint})
		if err != nil {
			return "", err
		}
	}
	volume.MountPoint = mountPoint
	if err := d.saveVolume(volume); err != nil {
		return "", err
	}

	return mountPoint, nil
}

func (d *Driver) putVolumeMountPoint(mountPoint string) {
	if strings.HasPrefix(mountPoint, filepath.Join(d.Root, MOUNTS_DIR)) {
		if err := os.Remove(mountPoint); err != nil {
			log.Warnf("Cannot cleanup mount point directory %v\n", mountPoint)
		}
	}
}

func (d *Driver) UmountVolume(id string) error {
	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return err
	}
	if volume.MountPoint == "" {
		log.Debug("Umount a umounted volume %v", id)
		return nil
	}
	if _, err := util.Execute(UMOUNT_BINARY, []string{volume.MountPoint}); err != nil {
		return err
	}
	d.putVolumeMountPoint(volume.MountPoint)

	volume.MountPoint = ""
	if err := d.saveVolume(volume); err != nil {
		return err
	}

	return nil
}

func (d *Driver) MountPoint(id string) (string, error) {
	volume, err := d.checkLoadVolume(id)
	if err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}
