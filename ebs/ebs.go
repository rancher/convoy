package ebs

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	. "github.com/rancher/convoy/logging"
)

const (
	DRIVER_NAME        = "ebs"
	DRIVER_CONFIG_FILE = "ebs.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"

	EBS_DEFAULT_VOLUME_SIZE = "ebs.defaultvolumesize"
	EBS_DEFAULT_VOLUME_TYPE = "ebs.defaultvolumetype"

	DEFAULT_VOLUME_SIZE = "4G"
	DEFAULT_VOLUME_TYPE = "gp2"

	MOUNTS_DIR    = "mounts"
	MOUNT_BINARY  = "mount"
	UMOUNT_BINARY = "umount"
)

type Driver struct {
	mutex      *sync.RWMutex
	ebsService *ebsService
	Device
}

type Device struct {
	Root              string
	DefaultVolumeSize int64
	DefaultVolumeType string
}

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
}

type Snapshot struct {
	UUID       string
	VolumeUUID string
	EBSID      string
}

type Volume struct {
	UUID       string
	EBSID      string
	Device     string
	MountPoint string
	Snapshots  map[string]Snapshot

	configPath string
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
}

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, CFG_PREFIX+VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
}

func init() {
	convoydriver.Register(DRIVER_NAME, Init)
}

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("ebs", fields, format, v)
}

func checkVolumeType(volumeType string) error {
	validVolumeType := map[string]bool{
		"gp2":      true,
		"io1":      true,
		"standard": true,
	}
	if !validVolumeType[volumeType] {
		return fmt.Errorf("Invalid volume type %v", volumeType)
	}
	return nil
}

func Init(root string, config map[string]string) (convoydriver.ConvoyDriver, error) {
	ebsService, err := NewEBSService()
	if err != nil {
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
	} else {
		if err := util.MkdirIfNotExists(root); err != nil {
			return nil, err
		}

		if config[EBS_DEFAULT_VOLUME_SIZE] == "" {
			config[EBS_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		size, err := util.ParseSize(config[EBS_DEFAULT_VOLUME_SIZE])
		if err != nil {
			return nil, err
		}
		if config[EBS_DEFAULT_VOLUME_TYPE] == "" {
			config[EBS_DEFAULT_VOLUME_TYPE] = DEFAULT_VOLUME_TYPE
		}
		volumeType := config[EBS_DEFAULT_VOLUME_TYPE]
		if err := checkVolumeType(volumeType); err != nil {
			return nil, err
		}
		dev = &Device{
			Root:              root,
			DefaultVolumeSize: size,
			DefaultVolumeType: volumeType,
		}
		if err := util.ObjectSave(dev); err != nil {
			return nil, err
		}
	}
	d := &Driver{
		mutex:      &sync.RWMutex{},
		ebsService: ebsService,
		Device:     *dev,
	}

	return d, nil
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) Info() (map[string]string, error) {
	infos := make(map[string]string)
	infos["DefaultVolumeSize"] = strconv.FormatInt(d.DefaultVolumeSize, 10)
	infos["DefaultVolumeType"] = d.DefaultVolumeType
	infos["InstanceID"] = d.ebsService.InstanceID
	infos["Region"] = d.ebsService.Region
	infos["AvailablityZone"] = d.ebsService.AvailabilityZone
	return infos, nil
}

func (d *Driver) VolumeOps() (convoydriver.VolumeOperations, error) {
	return d, nil
}

func (d *Driver) getSize(opts map[string]string) (int64, error) {
	size := opts[convoydriver.OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(d.DefaultVolumeSize, 10)
	}
	return util.ParseSize(size)
}

func (d *Driver) getType(opts map[string]string) (string, error) {
	volumeType := opts[convoydriver.OPT_VOLUME_TYPE]
	if volumeType == "" {
		volumeType = d.DefaultVolumeType
	}
	if err := checkVolumeType(volumeType); err != nil {
		return "", err
	}
	return volumeType, nil
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	var (
		err        error
		volumeSize int64
		format     bool
	)

	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	exists, err := util.ObjectExists(volume)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("Volume %v already exists", id)
	}

	//EBS volume ID
	volumeID := opts[convoydriver.OPT_VOLUME_ID]
	if volumeID == "" {
		// Create a new EBS volume
		volumeSize, err = d.getSize(opts)
		if err != nil {
			return err
		}
		volumeType, err := d.getType(opts)
		if err != nil {
			return err
		}
		volumeID, err = d.ebsService.CreateVolume(volumeSize, "", volumeType)
		if err != nil {
			return err
		}
		log.Debugf("Created volume %v from EBS volume %v", id, volumeID)
		format = true
	} else {
		ebsVolume, err := d.ebsService.GetVolume(volumeID)
		if err != nil {
			return err
		}
		volumeSize = *ebsVolume.Size * GB
		log.Debugf("Found EBS volume %v for volume %v", volumeID, id)
	}

	dev, err := d.ebsService.AttachVolume(volumeID, volumeSize)
	if err != nil {
		return err
	}
	log.Debugf("Attached EBS volume %v to %v", volumeID, dev)

	volume.EBSID = volumeID
	volume.Device = dev
	volume.Snapshots = make(map[string]Snapshot)

	// We don't format existing volume
	if format {
		if _, err := util.Execute("mkfs", []string{"-t", "ext4", dev}); err != nil {
			return err
		}
	}

	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(id string, opts map[string]string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if err := d.ebsService.DetachVolume(volume.EBSID); err != nil {
		return err
	}
	log.Debugf("Detached %v(%v) from %v", id, volume.EBSID, volume.Device)

	referenceOnly, _ := strconv.ParseBool(opts[convoydriver.OPT_REFERENCE_ONLY])
	if !referenceOnly {
		if err := d.ebsService.DeleteVolume(volume.EBSID); err != nil {
			return err
		}
		log.Debugf("Deleted %v(%v)", id, volume.EBSID)
	}
	return util.ObjectDelete(volume)
}

func mounted(dev, mountPoint string) bool {
	output, err := util.Execute(MOUNT_BINARY, []string{})
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
	dev := volume.Device
	specifiedPoint := opts[convoydriver.OPT_MOUNT_POINT]
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
		log.Debugf("Umount a umounted volume %v", id)
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
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return nil, err
	}

	ebsVolume, err := d.ebsService.GetVolume(volume.EBSID)
	if err != nil {
		return nil, err
	}

	info := map[string]string{
		"Device":          volume.Device,
		"MountPoint":      volume.MountPoint,
		"UUID":            volume.UUID,
		"EBSVolumeID":     volume.EBSID,
		"AvailablityZone": *ebsVolume.AvailabilityZone,
		"CreatedTime":     (*ebsVolume.CreateTime).Format(time.RubyDate),
		"Size":            strconv.FormatInt(*ebsVolume.Size*GB, 10),
		"State":           *ebsVolume.State,
		"Type":            *ebsVolume.VolumeType,
	}
	return info, nil
}

func (d *Driver) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(d.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
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

func (d *Driver) SnapshotOps() (convoydriver.SnapshotOperations, error) {
	return d, nil
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

func (d *Driver) CreateSnapshot(id, volumeID string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	volume := d.blankVolume(volumeID)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}
	snapshot, exists := volume.Snapshots[id]
	if exists {
		return generateError(logrus.Fields{
			LOG_FIELD_VOLUME:   volumeID,
			LOG_FIELD_SNAPSHOT: id,
		}, "Already has snapshot with uuid")
	}

	desc := fmt.Sprintf("Convoy snapshot %v for volume %v", id, volumeID)
	ebsSnapshotID, err := d.ebsService.CreateSnapshot(volume.EBSID, desc)
	if err != nil {
		return err
	}
	log.Debugf("Creating snapshot %v(%v) of volume %v(%v)", id, ebsSnapshotID, volumeID, volume.EBSID)

	snapshot = Snapshot{
		UUID:       id,
		VolumeUUID: volumeID,
		EBSID:      ebsSnapshotID,
	}
	volume.Snapshots[id] = snapshot
	return util.ObjectSave(volume)
}

func (d *Driver) DeleteSnapshot(id, volumeID string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	if err := d.ebsService.DeleteSnapshot(snapshot.EBSID); err != nil {
		return err
	}
	log.Debugf("Deleting snapshot %v(%v) of volume %v(%v)", id, snapshot.EBSID, volumeID, volume.EBSID)
	delete(volume.Snapshots, id)
	return util.ObjectSave(volume)
}

func (d *Driver) GetSnapshotInfo(id, volumeID string) (map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	snapshot, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}

	ebsSnapshot, err := d.ebsService.GetSnapshot(snapshot.EBSID)
	if err != nil {
		return nil, err
	}

	info := map[string]string{
		"UUID":          id,
		"VolumeUUID":    volumeID,
		"EBSSnapshotID": *ebsSnapshot.SnapshotId,
		"EBSVolumeID":   *ebsSnapshot.VolumeId,
		"StartTime":     (*ebsSnapshot.StartTime).Format(time.RubyDate),
		"Size":          strconv.FormatInt(*ebsSnapshot.VolumeSize*GB, 10),
		"State":         *ebsSnapshot.State,
	}

	return info, nil
}

func (d *Driver) ListSnapshot(opts map[string]string) (map[string]map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

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

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return d, nil
}

func checkEBSSnapshotID(id string) error {
	validID := regexp.MustCompile(`^snap-[0-9a-z]+$`)
	if !validID.MatchString(id) {
		return fmt.Errorf("Invalid EBS snapshot id %v", id)
	}
	return nil
}

func encodeURL(region, ebsSnapshotID string) string {
	return "ebs://" + region + "/" + ebsSnapshotID
}

func decodeURL(backupURL string) (string, string, error) {
	u, err := url.Parse(backupURL)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != DRIVER_NAME {
		return "", "", fmt.Errorf("BUG: Why dispatch %v to %v?", u.Scheme, DRIVER_NAME)
	}

	region := u.Host
	ebsSnapshotID := strings.TrimRight(strings.TrimLeft(u.Path, "/"), "/")
	if err := checkEBSSnapshotID(ebsSnapshotID); err != nil {
		return "", "", err
	}

	return region, ebsSnapshotID, nil
}

func (d *Driver) CreateBackup(snapshotID, volumeID, destURL string, opts map[string]string) (string, error) {
	//destURL is not necessary in EBS case
	snapshot, _, err := d.getSnapshotAndVolume(snapshotID, volumeID)
	if err != nil {
		return "", err
	}

	if err := d.ebsService.WaitForSnapshotComplete(snapshot.EBSID); err != nil {
		return "", err
	}
	return encodeURL(d.ebsService.Region, snapshot.EBSID), nil
}

func (d *Driver) DeleteBackup(backupURL string) error {
	//DeleteBackup is no-op in EBS
	return nil
}

func (d *Driver) GetBackupInfo(backupURL string) (map[string]string, error) {
	region, ebsSnapshotID, err := decodeURL(backupURL)
	if err != nil {
		return nil, err
	}
	ebsSnapshot, err := d.ebsService.GetSnapshotWithRegion(ebsSnapshotID, region)
	if err != nil {
		return nil, err
	}

	info := map[string]string{
		"EBSSnapshotID": *ebsSnapshot.SnapshotId,
		"EBSVolumeID":   *ebsSnapshot.VolumeId,
		"StartTime":     (*ebsSnapshot.StartTime).Format(time.RubyDate),
		"Size":          strconv.FormatInt(*ebsSnapshot.VolumeSize*GB, 10),
		"State":         *ebsSnapshot.State,
	}

	return info, nil
}

func (d *Driver) ListBackup(destURL string, opts map[string]string) (map[string]map[string]string, error) {
	//EBS doesn't support ListBackup(), return empty to satisfy caller
	return map[string]map[string]string{}, nil
}
