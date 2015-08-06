// +build linux

package devmapper

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/rancher/rancher-volume/objectstore"
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

	DEFAULT_VOLUME_SIZE = "100G"

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
	Mutex *sync.Mutex
	Device
}

type Volume struct {
	UUID       string
	DevID      int
	Size       int64
	Base       string
	MountPoint string
	Snapshots  map[string]Snapshot

	configPath string
}

type Snapshot struct {
	DevID     int
	Activated bool
}

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, DEVMAPPER_CFG_PREFIX+VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
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

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
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

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, DEVMAPPER_CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
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
		volume := d.blankVolume(id)
		if err := util.ObjectLoad(volume); err != nil {
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
		volume := d.blankVolume(uuid)
		if err := util.ObjectLoad(volume); err != nil {
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
	dev := &Device{
		Root: root,
	}
	exists, err := util.ObjectExists(dev)
	if err != nil {
		return nil, err
	}
	if exists {
		if err := util.ObjectLoad(dev); err != nil {
			return nil, err
		}
		d := &Driver{
			Mutex:  &sync.Mutex{},
			Device: *dev,
		}
		if err := d.activatePool(); err != nil {
			return nil, err
		}
		if err := d.remountVolumes(); err != nil {
			return nil, err
		}
		return d, nil
	}

	dev, err = verifyConfig(config)
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

	if err = createPool(filepath.Base(dev.ThinpoolDevice), dataDev, metadataDev, uint32(dev.ThinpoolBlockSize)); err != nil {
		return nil, err
	}

	if err = util.ObjectSave(dev); err != nil {
		return nil, err
	}
	d := &Driver{
		Mutex:  &sync.Mutex{},
		Device: *dev,
	}
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
	if err := util.ObjectSave(&d.Device); err != nil {
		return 0, err
	}
	return d.LastDevID, nil
}

func getSize(opts map[string]string) (int64, error) {
	size := opts[storagedriver.OPT_SIZE]
	if size == "" || size == "0" {
		size = DEFAULT_VOLUME_SIZE
	}
	return util.ParseSize(size)
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	var (
		size int64
		err  error
	)
	backupURL := opts[storagedriver.OPT_BACKUP_URL]
	if backupURL != "" {
		objVolume, err := objectstore.LoadVolume(backupURL)
		if err != nil {
			return err
		}
		size = objVolume.Size
	} else {
		size, err = getSize(opts)
		if err != nil {
			return err
		}
	}

	if size%(d.ThinpoolBlockSize*SECTOR_SIZE) != 0 {
		return fmt.Errorf("Size must be multiple of block size")

	}
	volume := d.blankVolume(id)
	exists, err := util.ObjectExists(volume)
	if err != nil {
		return err
	}
	if exists {
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

	volume.DevID = devID
	volume.Size = size
	volume.Snapshots = make(map[string]Snapshot)
	if err := util.ObjectSave(volume); err != nil {
		return err
	}

	dev, err := d.GetVolumeDevice(id)
	if err != nil {
		return err
	}
	if backupURL == "" {
		// format the device
		if _, err := util.Execute("mkfs", []string{"-t", "ext4", dev}); err != nil {
			return err
		}
	} else {
		if err := objectstore.RestoreDeltaBlockBackup(backupURL, dev); err != nil {
			return err
		}
	}

	return nil
}

func (d *Driver) DeleteVolume(id string) error {
	var err error
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
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

	if err := util.ObjectDelete(volume); err != nil {
		return err
	}
	return nil
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	volumes := make(map[string]map[string]string)
	volumeIDs, err := d.listVolumeIDs()
	if err != nil {
		return nil, err
	}
	for _, uuid := range volumeIDs {
		volumes[uuid], err = d.GetVolumeInfo(uuid)
		if err != nil {
			return nil, err
		}
	}
	return volumes, nil
}

func (d *Driver) CreateSnapshot(id, volumeID string) error {
	var err error

	volume := d.blankVolume(volumeID)
	if err := util.ObjectLoad(volume); err != nil {
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

	if err := util.ObjectSave(volume); err != nil {
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

	if err = util.ObjectSave(volume); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Info() (map[string]string, error) {
	// from sector count to byte
	blockSize := d.ThinpoolBlockSize * 512

	info := map[string]string{
		"Driver":            d.Name(),
		"Root":              d.Root,
		"DataDevice":        d.DataDevice,
		"MetadataDevice":    d.MetadataDevice,
		"ThinpoolDevice":    d.ThinpoolDevice,
		"ThinpoolSize":      strconv.FormatInt(d.ThinpoolSize, 10),
		"ThinpoolBlockSize": strconv.FormatInt(blockSize, 10),
	}

	return info, nil
}

func (d *Driver) getSnapshotAndVolume(snapshotID, volumeID string) (*Snapshot, *Volume, error) {
	volume := d.blankVolume(volumeID)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, nil, err
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
		volume := d.blankVolume(id)
		if err := util.ObjectLoad(volume); err != nil {
			return err
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
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}
	dev, err := d.GetVolumeDevice(id)
	if err != nil {
		return "", err
	}
	specifiedPoint := opts[storagedriver.OPT_MOUNT_POINT]
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
	if err := util.ObjectSave(volume); err != nil {
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
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
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
	if err := util.ObjectSave(volume); err != nil {
		return err
	}

	return nil
}

func (d *Driver) MountPoint(id string) (string, error) {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}
	return volume.MountPoint, nil
}

func (d *Driver) GetVolumeInfo(id string) (map[string]string, error) {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, err
	}
	result := map[string]string{
		"DevID": strconv.Itoa(volume.DevID),
		storagedriver.OPT_MOUNT_POINT: volume.MountPoint,
		storagedriver.OPT_SIZE:        strconv.FormatInt(volume.Size, 10),
	}
	return result, nil
}

func (d *Driver) GetSnapshotInfo(id, volumeID string) (map[string]string, error) {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}
	result := map[string]string{
		"UUID":       id,
		"VolumeUUID": volumeID,
		"DevID":      strconv.Itoa(snapshot.DevID),
		"Size":       strconv.FormatInt(volume.Size, 10),
	}
	log.Debug("Output result %v", result)
	return result, nil
}

func (d *Driver) ListSnapshot(opts map[string]string) (map[string]map[string]string, error) {
	var (
		volumeIDs []string
		err       error
	)
	snapshots := make(map[string]map[string]string)
	specifiedVolumeID := opts["VolumeID"]
	if specifiedVolumeID != "" {
		volumeIDs = []string{
			specifiedVolumeID,
		}
	} else {
		volumeIDs, err = d.listVolumeIDs()
		if err != nil {
			return nil, err
		}
	}
	for _, volumeID := range volumeIDs {
		volume := d.blankVolume(volumeID)
		if err := util.ObjectLoad(volume); err != nil {
			return nil, err
		}
		for snapshotID := range volume.Snapshots {
			snapshots[snapshotID], err = d.GetSnapshotInfo(snapshotID, volumeID)
			if err != nil {
				return nil, err
			}
		}
	}
	return snapshots, nil
}
