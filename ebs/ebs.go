package ebs

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/util"

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"

	"github.com/aws/aws-sdk-go/aws"
)

const (
	DRIVER_NAME        = "ebs"
	DRIVER_CONFIG_FILE = "ebs.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"

	EBS_DEFAULT_VOLUME_SIZE = "ebs.defaultvolumesize"
	EBS_DEFAULT_VOLUME_TYPE = "ebs.defaultvolumetype"
	EBS_DEFAULT_VOLUME_KEY  = "ebs.defaultkmskeyid"
	EBS_DEFAULT_ENCRYPTED   = "ebs.defaultencrypted"

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
	DefaultKmsKeyID   string
	DefaultEncrypted  bool
}

func (dev *Device) ConfigFile() (string, error) {
	if dev.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty device config path")
	}
	return filepath.Join(dev.Root, DRIVER_CONFIG_FILE), nil
}

type Snapshot struct {
	Name       string
	VolumeName string
	EBSID      string
}

type Volume struct {
	Name       string
	EBSID      string
	Device     string
	MountPoint string
	Snapshots  map[string]Snapshot

	configPath string
}

func (d *Driver) blankVolume(name string) *Volume {
	return &Volume{
		configPath: d.Root,
		Name:       name,
	}
}

func (v *Volume) ConfigFile() (string, error) {
	if v.Name == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume name")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, CFG_PREFIX+VOLUME_CFG_PREFIX+v.Name+CFG_POSTFIX), nil
}

func (v *Volume) GetDevice() (string, error) {
	return v.Device, nil
}

func (v *Volume) GetMountOpts() []string {
	return []string{}
}

func (v *Volume) GenerateDefaultMountPoint() string {
	return filepath.Join(v.configPath, MOUNTS_DIR, v.Name)
}

func init() {
	Register(DRIVER_NAME, Init)
}

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("ebs", fields, format, v)
}

func checkVolumeType(volumeType string) error {
	validVolumeType := map[string]bool{
		"gp2":      true,
		"io1":      true,
		"standard": true,
		"st1":      true,
		"sc1":      true,
	}
	if !validVolumeType[volumeType] {
		return fmt.Errorf("Invalid volume type %v", volumeType)
	}
	return nil
}

func (d *Driver) remountVolumes() error {
	volumeIDs, err := d.listVolumeNames()
	if err != nil {
		return err
	}
	for _, id := range volumeIDs {
		volume := d.blankVolume(id)
		if err := util.ObjectLoad(volume); err != nil {
			return err
		}
		if volume.MountPoint == "" {
			continue
		}
		req := Request{
			Name:    id,
			Options: map[string]string{},
		}
		if _, err := d.MountVolume(req); err != nil {
			return err
		}
	}
	return err
}

func Init(root string, config map[string]string) (ConvoyDriver, error) {
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
		kmsKeyId := config[EBS_DEFAULT_VOLUME_KEY]
		var encrypted bool
		if encryptedStr, ok := config[EBS_DEFAULT_ENCRYPTED]; ok {
			if encrypted, err = strconv.ParseBool(encryptedStr); err != nil {
				return nil, err
			}
		}
		dev = &Device{
			Root:              root,
			DefaultVolumeSize: size,
			DefaultVolumeType: volumeType,
			DefaultKmsKeyID:   kmsKeyId,
			DefaultEncrypted:  encrypted,
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
	if err := d.remountVolumes(); err != nil {
		return nil, err
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
	infos["DefaultKmsKey"] = d.DefaultKmsKeyID
	infos["DefaultEncrypted"] = fmt.Sprint(d.DefaultEncrypted)
	infos["InstanceID"] = d.ebsService.InstanceID
	infos["Region"] = d.ebsService.Region
	infos["AvailiablityZone"] = d.ebsService.AvailabilityZone
	return infos, nil
}

func (d *Driver) VolumeOps() (VolumeOperations, error) {
	return d, nil
}

func (d *Driver) getSize(opts map[string]string, defaultVolumeSize int64) (int64, error) {
	size := opts[OPT_SIZE]
	if size == "" || size == "0" {
		size = strconv.FormatInt(defaultVolumeSize, 10)
	}
	return util.ParseSize(size)
}

func (d *Driver) getTypeAndIOPS(opts map[string]string) (string, int64, error) {
	var (
		iops int64
		err  error
	)
	volumeType := opts[OPT_VOLUME_TYPE]
	if volumeType == "" {
		volumeType = d.DefaultVolumeType
	}
	if err := checkVolumeType(volumeType); err != nil {
		return "", 0, err
	}
	if opts[OPT_VOLUME_IOPS] != "" {
		iops, err = strconv.ParseInt(opts[OPT_VOLUME_IOPS], 10, 64)
		if err != nil {
			return "", 0, err
		}
	}
	if volumeType == "io1" && iops == 0 {
		return "", 0, fmt.Errorf("Invalid IOPS for volume type io1")
	}
	if volumeType != "io1" && iops != 0 {
		return "", 0, fmt.Errorf("IOPS only valid for volume type io1")
	}
	return volumeType, iops, nil
}

func (d *Driver) CreateVolume(req Request) error {
	var (
		err        error
		volumeSize int64
		format     bool
	)

	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	opts := req.Options

	volume := d.blankVolume(id)
	exists, err := util.ObjectExists(volume)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("Volume %v already exists", id)
	}

	//EBS volume ID
	volumeID := opts[OPT_VOLUME_DRIVER_ID]
	backupURL := opts[OPT_BACKUP_URL]
	if backupURL != "" && volumeID != "" {
		return fmt.Errorf("Cannot specify both backup and EBS volume ID")
	}

	newTags := map[string]string{
		"Name": id,
	}
	if volumeID != "" {
		ebsVolume, err := d.ebsService.GetVolume(volumeID)
		if err != nil {
			return err
		}
		volumeSize = *ebsVolume.Size * GB
		log.Debugf("Found EBS volume %v for volume %v, update tags", volumeID, id)
		if err := d.ebsService.AddTags(volumeID, newTags); err != nil {
			log.Debugf("Failed to update tags for volume %v, but continue", volumeID)
		}
	} else if backupURL != "" {
		region, ebsSnapshotID, err := decodeURL(backupURL)
		if err != nil {
			return err
		}
		if region != d.ebsService.Region {
			// We don't want to automatically copy snapshot here
			// because it's way too time consuming.
			return fmt.Errorf("Snapshot %v is at %v rather than current region %v. Copy snapshot is needed",
				ebsSnapshotID, region, d.ebsService.Region)
		}
		if err := d.ebsService.WaitForSnapshotComplete(ebsSnapshotID); err != nil {
			return err
		}
		log.Debugf("Snapshot %v is ready", ebsSnapshotID)
		ebsSnapshot, err := d.ebsService.GetSnapshot(ebsSnapshotID)
		if err != nil {
			return err
		}

		snapshotVolumeSize := *ebsSnapshot.VolumeSize * GB
		volumeSize, err = d.getSize(opts, snapshotVolumeSize)
		if err != nil {
			return err
		}
		if volumeSize < snapshotVolumeSize {
			return fmt.Errorf("Volume size cannot be less than snapshot size %v", snapshotVolumeSize)
		}
		volumeType, iops, err := d.getTypeAndIOPS(opts)
		if err != nil {
			return err
		}
		r := &CreateEBSVolumeRequest{
			Size:       volumeSize,
			SnapshotID: ebsSnapshotID,
			VolumeType: volumeType,
			IOPS:       iops,
			Tags:       newTags,
		}
		volumeID, err = d.ebsService.CreateVolume(r)
		if err != nil {
			return err
		}
		log.Debugf("Created volume %v from EBS snapshot %v", id, ebsSnapshotID)
	} else {

		// Create a new EBS volume
		volumeSize, err = d.getSize(opts, d.DefaultVolumeSize)
		if err != nil {
			return err
		}
		volumeType, iops, err := d.getTypeAndIOPS(opts)
		if err != nil {
			return err
		}
		r := &CreateEBSVolumeRequest{
			Size:       volumeSize,
			VolumeType: volumeType,
			IOPS:       iops,
			Tags:       newTags,
			KmsKeyID:   d.DefaultKmsKeyID,
		}
		volumeID, err = d.ebsService.CreateVolume(r)
		if err != nil {
			return err
		}
		log.Debugf("Created volume %s from EBS volume %v", id, volumeID)
		format = true
	}

	dev, err := d.ebsService.AttachVolume(volumeID, volumeSize)
	if err != nil {
		return err
	}
	log.Debugf("Attached EBS volume %v to %v", volumeID, dev)

	volume.Name = id
	volume.EBSID = volumeID
	volume.Device = dev
	volume.Snapshots = make(map[string]Snapshot)

	// We don't format existing or snapshot restored volume
	if format {
		if _, err := util.Execute("mkfs", []string{"-t", "ext4", dev}); err != nil {
			return err
		}
	}

	return util.ObjectSave(volume)
}

func (d *Driver) DeleteVolume(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	opts := req.Options

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return err
	}

	referenceOnly, _ := strconv.ParseBool(opts[OPT_REFERENCE_ONLY])
	if err := d.ebsService.DetachVolume(volume.EBSID); err != nil {
		if !referenceOnly {
			return err
		}
		//Ignore the error, remove the reference
		log.Warnf("Unable to detached %v(%v) due to %v, but continue with removing the reference",
			id, volume.EBSID, err)
	} else {
		log.Debugf("Detached %v(%v) from %v", id, volume.EBSID, volume.Device)
	}

	if !referenceOnly {
		if err := d.ebsService.DeleteVolume(volume.EBSID); err != nil {
			return err
		}
		log.Debugf("Deleted %v(%v)", id, volume.EBSID)
	}
	return util.ObjectDelete(volume)
}

func (d *Driver) MountVolume(req Request) (string, error) {
	id := req.Name
	opts := req.Options

	volume := d.blankVolume(id)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}

	mountPoint, err := util.VolumeMount(volume, opts[OPT_MOUNT_POINT], false)
	if err != nil {
		return "", err
	}

	if err := util.ObjectSave(volume); err != nil {
		return "", err
	}

	return mountPoint, nil
}

func (d *Driver) UmountVolume(req Request) error {
	id := req.Name

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

func (d *Driver) MountPoint(req Request) (string, error) {
	id := req.Name

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

	iops := ""
	if ebsVolume.Iops != nil {
		iops = strconv.FormatInt(*ebsVolume.Iops, 10)
	}
	info := map[string]string{
		"Device":                volume.Device,
		"MountPoint":            volume.MountPoint,
		"EBSVolumeID":           volume.EBSID,
		"KmsKeyId":              aws.StringValue(ebsVolume.KmsKeyId),
		"AvailiablityZone":      aws.StringValue(ebsVolume.AvailabilityZone),
		OPT_VOLUME_NAME:         id,
		OPT_VOLUME_CREATED_TIME: (*ebsVolume.CreateTime).Format(time.RubyDate),
		"Size":                  strconv.FormatInt(*ebsVolume.Size*GB, 10),
		"State":                 aws.StringValue(ebsVolume.State),
		"Type":                  aws.StringValue(ebsVolume.VolumeType),
		"IOPS":                  iops,
	}

	return info, nil
}

func (d *Driver) listVolumeNames() ([]string, error) {
	return util.ListConfigIDs(d.Root, CFG_PREFIX+VOLUME_CFG_PREFIX, CFG_POSTFIX)
}
func (d *Driver) ListVolume(opts map[string]string) (map[string]map[string]string, error) {
	volumes := make(map[string]map[string]string)
	volumeIDs, err := d.listVolumeNames()
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

func (d *Driver) SnapshotOps() (SnapshotOperations, error) {
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

func (d *Driver) CreateSnapshot(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeID, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return err
	}

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

	tags := map[string]string{
		"ConvoyVolumeName":   volumeID,
		"ConvoySnapshotName": id,
	}
	request := &CreateSnapshotRequest{
		VolumeID:    volume.EBSID,
		Description: fmt.Sprintf("Convoy snapshot"),
		Tags:        tags,
	}
	ebsSnapshotID, err := d.ebsService.CreateSnapshot(request)
	if err != nil {
		return err
	}
	log.Debugf("Creating snapshot %v(%v) of volume %v(%v)", id, ebsSnapshotID, volumeID, volume.EBSID)

	snapshot = Snapshot{
		Name:       id,
		VolumeName: volumeID,
		EBSID:      ebsSnapshotID,
	}
	volume.Snapshots[id] = snapshot
	return util.ObjectSave(volume)
}

func (d *Driver) DeleteSnapshot(req Request) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeID, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return err
	}

	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.Debugf("Removing reference of snapshot %v(%v) of volume %v(%v)", id, snapshot.EBSID, volumeID, volume.EBSID)
	delete(volume.Snapshots, id)
	return util.ObjectSave(volume)
}

func (d *Driver) GetSnapshotInfo(req Request) (map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	id := req.Name
	volumeID, err := util.GetFieldFromOpts(OPT_VOLUME_NAME, req.Options)
	if err != nil {
		return nil, err
	}

	return d.getSnapshotInfo(id, volumeID)
}

func (d *Driver) getSnapshotInfo(id, volumeID string) (map[string]string, error) {
	// Snapshot on EBS can be removed by DeleteBackup
	removed := false

	snapshot, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}

	ebsSnapshot, err := d.ebsService.GetSnapshot(snapshot.EBSID)
	if err != nil {
		removed = true
	}

	info := map[string]string{}
	if !removed {
		info = map[string]string{
			OPT_SNAPSHOT_NAME:         snapshot.Name,
			"VolumeName":              volumeID,
			"EBSSnapshotID":           aws.StringValue(ebsSnapshot.SnapshotId),
			"EBSVolumeID":             aws.StringValue(ebsSnapshot.VolumeId),
			"KmsKeyId":                aws.StringValue(ebsSnapshot.KmsKeyId),
			OPT_SNAPSHOT_CREATED_TIME: (*ebsSnapshot.StartTime).Format(time.RubyDate),
			OPT_SIZE:                  strconv.FormatInt(*ebsSnapshot.VolumeSize*GB, 10),
			"State":                   aws.StringValue(ebsSnapshot.State),
		}
	} else {
		info = map[string]string{
			OPT_SNAPSHOT_NAME: snapshot.Name,
			"VolumeName":      volumeID,
			"State":           "removed",
		}
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
	specifiedVolumeID, _ := util.GetFieldFromOpts(OPT_VOLUME_NAME, opts)
	if specifiedVolumeID != "" {
		volumeIDs = []string{
			specifiedVolumeID,
		}
	} else {
		volumeIDs, err = d.listVolumeNames()
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
			snapshots[snapshotID], err = d.getSnapshotInfo(snapshotID, volumeID)
			if err != nil {
				return nil, err
			}
		}
	}
	return snapshots, nil
}

func (d *Driver) BackupOps() (BackupOperations, error) {
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
	// Would remove the snapshot
	region, ebsSnapshotID, err := decodeURL(backupURL)
	if err != nil {
		return err
	}
	if err := d.ebsService.DeleteSnapshotWithRegion(ebsSnapshotID, region); err != nil {
		return err
	}
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
		"Region":        region,
		"EBSSnapshotID": aws.StringValue(ebsSnapshot.SnapshotId),
		"EBSVolumeID":   aws.StringValue(ebsSnapshot.VolumeId),
		"KmsKeyId":      aws.StringValue(ebsSnapshot.KmsKeyId),
		"StartTime":     (*ebsSnapshot.StartTime).Format(time.RubyDate),
		"Size":          strconv.FormatInt(*ebsSnapshot.VolumeSize*GB, 10),
		"State":         aws.StringValue(ebsSnapshot.State),
	}

	return info, nil
}

func (d *Driver) ListBackup(destURL string, opts map[string]string) (map[string]map[string]string, error) {
	//EBS doesn't support ListBackup(), return empty to satisfy caller
	return map[string]map[string]string{}, nil
}
