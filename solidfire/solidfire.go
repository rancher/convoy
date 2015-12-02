package solidfire

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
	"path/filepath"
	"strconv"
	"time"

	. "github.com/rancher/convoy/logging"
)

const (
	DRIVER_NAME        = "solidfire"
	DRIVER_CONFIG_FILE = "solidfire.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"

	DEFAULT_VOLUME_SIZE = "1G"
	DEFAULT_VOLUME_TYPE = "silver"

	SF_ENDPOINT            = "solidfire.endpoint"
	SF_DEFAULT_ACCOUNT_ID  = "solidfire.defaultaccountid"
	SF_SVIP                = "solidfire.svip"
	SF_DEFAULT_VOLUME_SIZE = "solidfire.defaultvolumesize"
	SF_DEFAULT_VOLUME_TYPE = "solidfire.defaultvolumetype"

	MOUNTS_DIR     = "mounts"
	MOUNTS_BINARY  = "mount"
	UNMOUNT_BINARY = "umount"
)

type Device struct {
	Root                        string
	DefaultVolumeSize           int64
	DefaultVolumeType           string
	SolidFireDefaultAccountID   int64
	SolidFireDefaultAccessGroup int64
	SolidFireEndpoint           string
	SolidFireSVIP               string
	SolidFireVolumeTypes        map[string]QoS
}

type Driver struct {
	Client Client
	Device
}

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "devmapper"})
)

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, CFG_PREFIX+VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
}

func (d *Driver) getSize(opts map[string]string, defaultVolumeSize int64) (int64, error) {
	size := opts[convoydriver.OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(defaultVolumeSize, 10)
	}
	return util.ParseSize(size)
}

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
}

func (v *Volume) GetDevice() (string, error) {
	return v.Device, nil
}

func (v *Volume) GetMountOpts() []string {
	return []string{}
}

func (v *Volume) GenerateDefaultMountPoint() string {
	return filepath.Join(v.configPath, MOUNTS_DIR, v.UUID)
}

func init() {
	convoydriver.Register(DRIVER_NAME, Init)
}

func (device *Device) listVolumeIDs() ([]string, error) {
	return util.ListConfigIDs(device.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}

func Init(root string, config map[string]string) (convoydriver.ConvoyDriver, error) {
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

		if config[SF_DEFAULT_VOLUME_SIZE] == "" {
			config[SF_DEFAULT_VOLUME_SIZE] = DEFAULT_VOLUME_SIZE
		}
		log.Error("parse sf default vol size")
		size, err := util.ParseSize(config[SF_DEFAULT_VOLUME_SIZE])
		if err != nil {
			return nil, err
		}
		if config[SF_DEFAULT_VOLUME_TYPE] == "" {
			config[SF_DEFAULT_VOLUME_TYPE] = DEFAULT_VOLUME_TYPE
		}
		volumeType := config[SF_DEFAULT_VOLUME_TYPE]

		log.Error("Try and set it")
		dev = &Device{
			Root:              root,
			DefaultVolumeSize: size,
			DefaultVolumeType: volumeType,
		}
		if err := util.ObjectSave(dev); err != nil {
			return nil, err
		}
	}
	log.Debugf("Gold type is: %v", dev.SolidFireVolumeTypes["Gold"])
	client := NewClient(dev.SolidFireEndpoint, dev.SolidFireSVIP, dev.DefaultVolumeSize, dev.SolidFireDefaultAccountID)
	d := &Driver{
		Device: *dev,
		Client: *client,
	}
	return d, nil
}

func (d *Driver) Info() (map[string]string, error) {
	infos := make(map[string]string)
	infos["DefaultVolumeSize"] = strconv.FormatInt(d.DefaultVolumeSize, 10)
	infos["DefaultVolumeType"] = d.DefaultVolumeType
	return infos, nil
}

func (d *Driver) VolumeOps() (convoydriver.VolumeOperations, error) {
	return d, nil
}

func checkVolumeType(volumeType string) error {
	validVolumeType := map[string]bool{
		"bronze":   true,
		"silver":   true,
		"gold":     true,
		"platinum": true,
	}
	if !validVolumeType[volumeType] {
		return fmt.Errorf("Invalid volume type %v", volumeType)
	}
	return nil
}

type Volume struct {
	UUID       string
	SFID       int64
	NAME       string
	Device     string
	MountPoint string
	Path       string
	Snapshots  map[string]Snapshot

	configPath string
}

type Snapshot struct {
	UUID       string
	SFID       int64
	SFVolID    int64
	VolumeUUID string
}

func (d *Driver) blankVolume(id string) *Volume {
	return &Volume{
		configPath: d.Root,
		UUID:       id,
	}
}

func (d *Driver) Name() string {
	return DRIVER_NAME
}

func (d *Driver) getVolumeType(opts map[string]string) (QoS, error) {
	//TODO(jdg): setup QoS, do we want to use types, or ditch this and just take IOP options?
	return QoS{}, nil
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

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("solidfire", fields, format, v)
}

func (d *Driver) CreateVolume(id string, opts map[string]string) error {
	var v SFVolume
	volume := d.blankVolume(id)
	exists, err := util.ObjectExists(volume)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("volume %v already exists", id)
	}

	size, err := d.getSize(opts, d.DefaultVolumeSize)
	if err != nil {
		return err
	}

	vType := opts[convoydriver.OPT_VOLUME_TYPE]
	if vType == "" {
		vType = d.DefaultVolumeType
	}

	var qos QoS
	qos = d.SolidFireVolumeTypes[vType]

	volID := opts[convoydriver.OPT_VOLUME_ID]
	backupURL := opts[convoydriver.OPT_BACKUP_URL]
	if backupURL != "" && volID != "" {
		return fmt.Errorf("Cannot specify both backup and SolidFire volume ID")
	} else if volID != "" {
		vid, err := strconv.ParseInt(volID, 10, 64)
		if err != nil {
			return err
		}
		req := &CloneVolumeRequest{
			Name:     id,
			VolumeID: vid,
		}
		v, err = d.Client.CloneVolume(req)
	} else if backupURL != "" {
		return fmt.Errorf("Create Volume from backup not currently implemented")
	} else {

		req := &CreateVolumeRequest{
			Name:      id,
			AccountID: d.SolidFireDefaultAccountID,
			TotalSize: size,
			Qos:       qos,
		}
		v, err = d.Client.CreateVolume(req)
	}

	vids := []int64{v.VolumeID}
	err = d.Client.AddVolumesToAccessGroup(d.SolidFireDefaultAccessGroup, vids)
	if err != nil {
		log.Debug("Cleaning up volume after failure to add to VAG")
		d.Client.DeleteVolume(v.VolumeID)
		log.Errorf("Failed to add volume %v to VAG %v (error: %v)", v.VolumeID, d.SolidFireDefaultAccessGroup, err)
		return err
	}
	format := true
	path, dev, err := d.Client.AttachVolume(v.VolumeID, "")
	volume.Device = dev
	if err != nil {
		log.Errorf("Failed to attach volume: %v", err)
		d.Client.DeleteVolume(v.VolumeID)
		return err
	}
	log.Debugf("Attached volume %v at disk %v as device %v", v.VolumeID, path, dev)
	if format {
		time.Sleep(5)
		_, err := util.Execute("create_part", []string{dev})
		if err != nil {
			d.Client.DeleteVolume(v.VolumeID)
			return err
		}
		dev += "1"
		if _, err := util.Execute("mkfs", []string{"-t", "ext4", dev}); err != nil {
			d.Client.DeleteVolume(v.VolumeID)
			return err
		}
		volume.Device = dev
	}
	volume.SFID = v.VolumeID
	volume.Snapshots = make(map[string]Snapshot)
	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(id string, opts map[string]string) error {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	referenceOnly, _ := strconv.ParseBool(opts[convoydriver.OPT_REFERENCE_ONLY])
	if err := d.Client.DetachVolume(volume.SFID, ""); err != nil {
		if !referenceOnly {
			return err
		}
		//Ignore the error,
		//remove the reference
		log.Warnf("Unable to detach %v(%v) due to %v, but continue with removing the reference",
			id, volume.SFID, err)
	} else {
		log.Debugf("Detached %v(%v) from %v", id, volume.SFID, volume.Device)
	}

	if !referenceOnly {
		if err := d.Client.DeleteVolume(volume.SFID); err != nil {
			return err
		}
		log.Debugf("Deleted %v(%v)", id, volume.SFID)
	}
	return util.ObjectDelete(volume)
}

func (d *Driver) MountVolume(id string, opts map[string]string) (string, error) {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}

	mountPoint, err := util.VolumeMount(volume, opts[convoydriver.OPT_MOUNT_POINT], false)
	// mountPoint, err := util.VolumeMount(volume, opts[convoydriver.OPT_MOUNT_POINT])
	if err != nil {
		return "", err
	}

	if err := util.ObjectSave(volume); err != nil {
		return "", err
	}

	return mountPoint, nil
}

func (d *Driver) UmountVolume(id string) error {
	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	if err := util.VolumeUmount(volume); err != nil {
		return err
	}

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

	sfVolume, err := d.Client.GetVolume(volume.SFID, "")
	if err != nil {
		return nil, err
	}
	//TODO(jdg): Add QoS to volumeInfo
	//qos := sfVolume.Qos
	info := map[string]string{
		"Device":      volume.Device,
		"MountPoint":  volume.MountPoint,
		"UUID":        volume.UUID,
		"SFID":        strconv.FormatInt(sfVolume.VolumeID, 10),
		"CreatedTime": sfVolume.CreateTime,
		"Size":        strconv.FormatInt(sfVolume.TotalSize, 10),
		"State":       sfVolume.Status,
	}
	return info, nil
}

func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	volumeIDs, err := d.listVolumeIDs()
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]string{}
	for _, id := range volumeIDs {
		result[id], err = d.GetVolumeInfo(id)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (d *Driver) SnapshotOps() (convoydriver.SnapshotOperations, error) {
	return d, nil
}

func (d *Driver) CreateSnapshot(id, volumeID string) error {
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

	req := &CreateSnapshotRequest{
		Name:     id,
		VolumeID: volume.SFID,
	}
	s, err := d.Client.CreateSnapshot(req)
	if err != nil {
		return err
	}
	log.Debugf("Creating snapshot reocrd %v(%v) of volume %v(%v)", id, s.SnapshotID, volumeID, volume.SFID)
	snapshot = Snapshot{
		UUID:       id,
		VolumeUUID: volumeID,
		SFID:       s.SnapshotID,
		SFVolID:    volume.SFID,
	}
	volume.Snapshots[id] = snapshot
	return util.ObjectSave(volume)
}

func (d *Driver) DeleteSnapshot(snapshotID, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(snapshotID, volumeID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotID,
		LOG_FIELD_VOLUME:   volumeID,
	}).Debugf("Deleting snapshot for volume")
	err = d.Client.DeleteSnapshot(snapshot.SFID)
	if err != nil {
		return err
	}
	log.Debug("Deleted snapshot device")
	delete(volume.Snapshots, snapshotID)
	if err = util.ObjectSave(volume); err != nil {
		return err
	}
	return nil
}

func (d *Driver) GetSnapshotInfo(id, volumeID string) (map[string]string, error) {
	snapshot, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}
	s, err := d.Client.GetSnapshot(snapshot.SFID, "")
	if err != nil {
		return nil, err
	}

	result := map[string]string{
		"UUID":        id,
		"VolumeUUID":  volumeID,
		"SolidFireID": strconv.FormatInt(snapshot.SFID, 10),
		"Size":        strconv.FormatInt(s.TotalSize, 10),
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

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return nil, fmt.Errorf("Backup operations not implemented yet")
}
