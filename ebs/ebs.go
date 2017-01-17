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

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"
	"github.com/rancher/convoy/util"
	"github.com/rancher/convoy/util/fs"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

const (
	DRIVER_NAME        = "ebs"
	DRIVER_CONFIG_FILE = "ebs.cfg"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_PREFIX        = DRIVER_NAME + "_"
	CFG_POSTFIX       = ".json"

	EBS_DEFAULT_VOLUME_SIZE = "ebs.defaultvolumesize"
	EBS_DEFAULT_VOLUME_TYPE = "ebs.defaultvolumetype"
	EBS_CLUSTER_NAME = "ebs.clustername"
	EBS_DEFAULT_VOLUME_KEY  = "ebs.defaultkmskeyid"
	EBS_DEFAULT_ENCRYPTED   = "ebs.defaultencrypted"
	EBS_DEFAULT_FILESYSTEM  = "ebs.defaultfilesystem"
	EBS_AUTOFORMAT  = "ebs.autoformat"
	EBS_AUTORESIZEFS  = "ebs.autoresizefs"

	DEFAULT_VOLUME_SIZE = "4G"
	DEFAULT_VOLUME_TYPE = "gp2"
	DEFAULT_CLUSTER_NAME = ""
	DEFAULT_FILESYSTEM   = "ext4"

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
	DefaultDCName     string
	DefaultFSType     string
	DefaultKmsKeyID   string
	DefaultEncrypted  bool
	AutoResizeFS      bool
	AutoFormat        bool
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

func getTagValue(key string, tags []*ec2.Tag) string {
	for _, tag := range tags {
		if key == *tag.Key {
			return *tag.Value
		}
	}
	return ""
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
		if config[EBS_CLUSTER_NAME] == "" {
			config[EBS_CLUSTER_NAME] = DEFAULT_CLUSTER_NAME
		}
		log.Debugf("Setting DC name in driver as %s", config[EBS_CLUSTER_NAME])
		dcName := config[EBS_CLUSTER_NAME]
		if config[EBS_DEFAULT_FILESYSTEM] == "" {
			config[EBS_DEFAULT_FILESYSTEM] = DEFAULT_FILESYSTEM
		} else {
			log.Debugf("Setting default filesystem type in driver to %q", config[EBS_DEFAULT_FILESYSTEM])
		}
		fsType := config[EBS_DEFAULT_FILESYSTEM]
		kmsKeyId := config[EBS_DEFAULT_VOLUME_KEY]
		var encrypted bool
		if encryptedStr, ok := config[EBS_DEFAULT_ENCRYPTED]; ok {
			if encrypted, err = strconv.ParseBool(encryptedStr); err != nil {
				return nil, err
			}
		}

		autoFormat := true
		autoResizefs := true
		if autoFormatStr, ok := config[EBS_AUTOFORMAT]; ok{
			if autoFormat, err = strconv.ParseBool(autoFormatStr); err != nil {
				return nil, err
			}
		}
		if autoResizeStr, ok := config[EBS_AUTORESIZEFS]; ok{
			if autoResizefs, err = strconv.ParseBool(autoResizeStr); err != nil {
				return nil, err
			}
		}

		dev = &Device{
			Root:              root,
			DefaultVolumeSize: size,
			DefaultVolumeType: volumeType,
			DefaultDCName: dcName,
			DefaultFSType:     fsType,
			DefaultKmsKeyID:   kmsKeyId,
			DefaultEncrypted:  encrypted,
			AutoFormat: autoFormat,
			AutoResizeFS: autoResizefs,
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
	infos["DefaultFSType"] = d.DefaultFSType
	infos["DefaultEncrypted"] = fmt.Sprint(d.DefaultEncrypted)
	infos["InstanceID"] = d.ebsService.InstanceID
	infos["Region"] = d.ebsService.Region
	infos["AvailiablityZone"] = d.ebsService.AvailabilityZone
	infos["AutoResizeFS"] = d.AutoResizeFS
	infos["AutoFormat"] = d.AutoFormat
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

func convertEc2TagsToMap(tags []*ec2.Tag) map[string]string {
	tagMap := make(map[string]string)
	for _, tag := range tags {
		tagMap[*tag.Key] = *tag.Value
	}
	return tagMap
}

func (d *Driver) MarkVolumeForGC(volume *ec2.Volume) error {
	if volume == nil {
		return nil
	}
	gcTag := make(map[string]string)
	gcTag["GarbageCollection"] = time.Now().String()
	if err := d.ebsService.AddTags(*volume.VolumeId, gcTag); err != nil {
		return err
	}
	return nil
}

func (d *Driver) CreateAndBuildFromSnapshot(oldVolume *ec2.Volume, snapshotVolume *ec2.Volume, opts map[string]string) (string, int64, error) {
	// If the volume I would snapshot off of is the same as the one that I need to create then just return
	if oldVolume != nil && *oldVolume.VolumeId == *snapshotVolume.VolumeId {
		return *oldVolume.VolumeId, *oldVolume.Size * GB, nil
	}
	csr := &CreateSnapshotRequest {
		VolumeID: *snapshotVolume.VolumeId,
		Description: fmt.Sprintf("Creating snaphot from %s", *snapshotVolume.VolumeId),
		Tags: convertEc2TagsToMap(snapshotVolume.Tags),
	}
	// Create snapshot from volume in other AZ
	snapshotId, err := d.ebsService.CreateSnapshot(csr)
	if err != nil {
		return "", -1, err
	}

	snapshot, err := d.ebsService.GetSnapshot(snapshotId)
	if err != nil {
		return "", -1, err
	}
	return d.BuildFromSnapshot(oldVolume, snapshot, opts)
}

func (d *Driver) BuildFromSnapshot(oldVolume *ec2.Volume, snapshot *ec2.Snapshot, opts map[string]string) (string, int64, error) {
	// If the snapshot to rebuild off of was created from the oldVolume then update tags and move on
	if oldVolume != nil && *oldVolume.VolumeId == *snapshot.VolumeId {
		return *oldVolume.VolumeId, *oldVolume.Size * GB, nil
	}

	// If there is an old volume then we will mark is for GarbageCollection
	if err := d.MarkVolumeForGC(oldVolume); err != nil {
		return "", -1, err
	}

	if err := d.ebsService.WaitForSnapshotComplete(*snapshot.SnapshotId); err != nil {
		return "", -1, err
	}
	log.Debugf("Snapshot %v is ready", *snapshot.SnapshotId)
	snapshotVolumeSize := *snapshot.VolumeSize * GB
	volumeSize, err := d.getSize(opts, snapshotVolumeSize)
	if err != nil {
		return "", volumeSize, err
	}
	if volumeSize < snapshotVolumeSize {
		return "", volumeSize, fmt.Errorf("Volume size cannot be less than snapshot size %v", snapshotVolumeSize)
	}

	volumeType, iops, err := d.getTypeAndIOPS(opts)
	if err != nil {
		return "", volumeSize, err
	}

	r := &CreateEBSVolumeRequest{
		Size:       volumeSize,
		SnapshotID: *snapshot.SnapshotId,
		VolumeType: volumeType,
		IOPS:       iops,
		Tags:       convertEc2TagsToMap(snapshot.Tags),
		Encrypted:  *snapshot.Encrypted,
	}

	volumeID, err := d.ebsService.CreateVolume(r)
	if err != nil {
		return volumeID, volumeSize, err
	}
	log.Debugf("Created volume %v from EBS snapshot %v", volumeID, *snapshot.SnapshotId)
	return volumeID, volumeSize, nil
}

func (d *Driver) UpdateTags(volumeID string, newTags map[string]string) error {
	if err := d.ebsService.AddTags(volumeID, newTags); err != nil {
		log.Debugf("Failed to update tags for volume %v, but continue", volumeID)
		return err
	}
	return nil
}

func (d *Driver) FailoverLogic(volumeName string, volumeID string, opts map[string]string, newTags map[string]string, needsFS *bool) (string, int64, error) {

	var volumeSize int64
	liveAvailabilityZones, err := d.ebsService.GetAvailabilityZones(
		&ec2.Filter {
			Name: aws.String("state"),
			Values: []*string {
				aws.String("available"),
			},
		},
	)
	if err != nil {
		return volumeID, volumeSize, err
	} else if len(liveAvailabilityZones) == 0 {
		return volumeID, volumeSize, fmt.Errorf("AWS is reporting not any Availablity Zones in \"available\" state")
	}

	// Need to specify the volume and the filter for availability-zones
	mostRecentVolume, err := d.ebsService.GetMostRecentVolume(
		volumeName, 
		d.DefaultDCName,
		&ec2.Filter {
			Name: aws.String("availability-zone"),
			Values: liveAvailabilityZones,
		},
	)
	if err != nil {
		return volumeID, volumeSize, err
	}

	mostRecentSnapshot, err := d.ebsService.GetMostRecentSnapshot(volumeName, d.DefaultDCName)
	if err != nil {
		return volumeID, volumeSize, err
	}

	// if volumeID is empty and most recent volume is empty then blow up
	if volumeID != "" && mostRecentVolume == nil {
		return volumeID, volumeSize, fmt.Errorf("Most recent volume was nil for volumeID: %s", volumeID)
	}

	var oldVolume *ec2.Volume
	if volumeID != "" {
		if oldVolume, err = d.ebsService.GetVolume(volumeID); err != nil {
			return volumeID, volumeSize, err
		}
		// If Failover is false then return 
		if getTagValue("Failover", oldVolume.Tags) != "False" {
			return *oldVolume.VolumeId, *oldVolume.Size * GB, nil
		}

	}

	// Both are nil, create new volume from scratch
	if mostRecentSnapshot == nil && mostRecentVolume == nil {
		volumeSize, err := d.getSize(opts, d.DefaultVolumeSize)
		if err != nil {
			return volumeID, volumeSize, err
		}
		volumeType, iops, err := d.getTypeAndIOPS(opts)
		if err != nil {
			return volumeID, volumeSize, err
		}
		r := &CreateEBSVolumeRequest{
			Size:       volumeSize,
			VolumeType: volumeType,
			IOPS:       iops,
			KmsKeyID:   d.DefaultKmsKeyID,
			Encrypted:  d.DefaultEncrypted,
		}
		volumeID, err = d.ebsService.CreateVolume(r)
		if err != nil {
			return volumeID, volumeSize, err
		}
		log.Debugf("Created volume %s from EBS volume %v", volumeName, volumeID)
		needsFS = aws.Bool(true)
		return volumeID, volumeSize, nil
	} else if mostRecentSnapshot == nil {
		// If snapshot is nil then rebuild from most recent volume
		return d.CreateAndBuildFromSnapshot(oldVolume, mostRecentVolume, opts)
	} else if mostRecentVolume == nil {
		return d.BuildFromSnapshot(oldVolume, mostRecentSnapshot, opts)
	} else if (*mostRecentVolume.CreateTime).After(*mostRecentSnapshot.StartTime) {
		return d.CreateAndBuildFromSnapshot(oldVolume, mostRecentVolume, opts)
	} else {
		// If we branched here it means the mostRecentSnapshot is newer than the most recent volume
		if (*mostRecentVolume.VolumeId) == (*mostRecentSnapshot.VolumeId) {
			// If the most recent snapshot is based off the most recent volume, then the volume could have more up to date data so build from it
			return d.CreateAndBuildFromSnapshot(oldVolume, mostRecentVolume, opts)
		} else {
			// The snapshot is from a volume that no longer exists so build from it
			return d.BuildFromSnapshot(oldVolume, mostRecentSnapshot, opts)
		}
	}

}

func (d *Driver) CreateVolume(req Request) error {
	var (
		err        error
		volumeSize int64
		needsFS    bool
	)

	d.mutex.Lock()
	defer d.mutex.Unlock()

	log.Debugf("Create volume request object: %v", req)
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

	//check if the volume name is a 64 chars alphanumeric string.
	//Since this might imply a random name generated by Docker.
	//This check prevents run-away volume creation in EBS
	re := regexp.MustCompile("^[a-zA-Z0-9]{64}$")
	if re.MatchString(id){
		return fmt.Errorf("Volume name 64 chars alphanumeric. Are you sure this is not a Docker generated volume name? Please change the name and try again.")
	}

	//EBS volume name
	volumeName := opts[OPT_VOLUME_NAME]
	//EBS volume ID
	volumeID := opts[OPT_VOLUME_DRIVER_ID]
	backupURL := opts[OPT_BACKUP_URL]
	if backupURL != "" && volumeID != "" {
		return fmt.Errorf("Cannot specify both backup and EBS volume ID")
	}
	if volumeID != "" && volumeName != "" {
		return fmt.Errorf("Cannot specify both EBS volume ID and EBS volume Name")
	}
	if volumeName != "" {
		log.Debugf("Looking up volume by name %s", volumeName)
		ebsVolume, err := d.ebsService.GetVolumeByName(volumeName, d.DefaultDCName)
		if err == nil {
			volumeSize = *ebsVolume.Size * GB
			volumeID = aws.StringValue(ebsVolume.VolumeId)
			log.Debugf("Found EBS volume %v with name %v", volumeID, volumeName)
		}
	}

	log.Debugf("Creating volume %s for cluster %s in AZ %s", id, d.DefaultDCName, d.ebsService.AvailabilityZone)
	newTags := map[string]string{
		"Name": id,
		"DCName": d.DefaultDCName,
	}

	// If Failover Tag is false, will be designated inside this logic and return the proper values
	volumeID, volumeSize, err = d.FailoverLogic(volumeName, volumeID, opts, newTags, &needsFS)
	if err != nil {
		return err
	}
	d.UpdateTags(volumeID, newTags)

	dev, err := d.ebsService.AttachVolume(volumeID, volumeSize)
	if err != nil {
		return err
	}
	log.Debugf("Attached EBS volume %v to %v", volumeID, dev)

	volume.Name = id
	volume.EBSID = volumeID
	volume.Device = dev
	volume.Snapshots = make(map[string]Snapshot)

	if !needsFS {
		if fsType, err := fs.Detect(volume.Device); err != nil {
			if err == fs.ErrNoFilesystemDetected {
				needsFS = true
			} else {
				return err
			}
		} else {
			log.Debugf("Detected existing filesystem type=%s for device=%s", fsType, volume.Device)
			if d.AutoResizeFS {
				log.Debugf("Ensuring filesystem size and device's (%s) size are in sync.", volume.Device)
				if er := fs.Resize(volume.Device); er != nil {
					log.Debugf("Error in syncing sizes %s", volume.Device)
					return er
				}
			}
		}
	}

	if needsFS {
		if d.AutoFormat {
			log.Debugf("Formatting device=%s with filesystem type=%s", volume.Device, d.DefaultFSType)
			if err := fs.FormatDevice(volume.Device, d.DefaultFSType); err != nil {
				return err
			}
		}
	}

	if err := util.ObjectSave(volume); err != nil {
		return err
	}

	return nil
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
		log.Warnf("Unable to detach %v(%v) Backend Error: %v, Deleting object from state",
			id, volume.EBSID, err)
	} else {
		log.Debugf("Detached %v(%v) from %v", id, volume.EBSID, volume.Device)
	}

	// Don't delete as per Medallia design, just remove reference
	/*
	if !referenceOnly {
		if err := d.ebsService.DeleteVolume(volume.EBSID); err != nil {
			return err
		}
		log.Debugf("Deleted %v(%v)", id, volume.EBSID)
	}
	*/
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
		// if device doesn't exist, it's a stale entry. Delete from state
		errorStr := err.Error()
		var validID = regexp.MustCompile(`. output mount: special device /dev/([a-z]+) does not exist`)
		if validID.MatchString(errorStr){
			util.ObjectDelete(volume)
		}
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

	//Medallia specific, unmount docker volume also implies a detach
	detachErr := d.DeleteVolume(req)
	if detachErr != nil{
		return detachErr
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

func (d *Driver) GetVolumesInfo(ids []string) ([]map[string]string, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	log.Debugf("Will be looking for ids of names: %s", ids)

	var ebsIDs []string
	var volumeObjects []*Volume
	for _, id := range ids{
		volume := d.blankVolume(id)
		if err := util.ObjectLoad(volume); err != nil {
			return nil, err
		}
		ebsIDs = append(ebsIDs, volume.EBSID)
		volumeObjects = append(volumeObjects, volume)
	}

	log.Debugf("Got IDS: %s", ebsIDs)

	var wg sync.WaitGroup
	ebsVolumes := make([]*ec2.Volume, len(ebsIDs))

	for k, ebsID := range ebsIDs{
		wg.Add(1)
		go func(i int, id string){
			defer wg.Done()
			ebsVolume, er := d.ebsService.GetVolume(id)
			if er != nil {
				if strings.Contains(er.Error(), "InvalidVolume.NotFound") {
					log.Debugf("Found a non-existent volume %s in state, deleting.", ids[i])
					vol := d.blankVolume(ids[i])
					util.ObjectLoad(vol)
					util.ObjectDelete(vol)
				}
			}else{
				ebsVolumes[i] = ebsVolume
			}
		}(k, ebsID)
	}
	wg.Wait()

	var infoList []map[string]string
	for i, ebsVolume := range ebsVolumes{
		if ebsVolume == nil{
			continue
		}
		iops := ""
		if ebsVolume.Iops != nil {
			iops = strconv.FormatInt(*ebsVolume.Iops, 10)
		}

		info := map[string]string{
			"Device":                volumeObjects[i].Device,
			"MountPoint":            volumeObjects[i].MountPoint,
			"EBSVolumeID":           volumeObjects[i].EBSID,
			"AvailiablityZone":      aws.StringValue(ebsVolume.AvailabilityZone),
			OPT_VOLUME_NAME:         volumeObjects[i].Name,
			OPT_VOLUME_CREATED_TIME: (*ebsVolume.CreateTime).Format(time.RubyDate),
			"Size":                  strconv.FormatInt(*ebsVolume.Size*GB, 10),
			"State":                 aws.StringValue(ebsVolume.State),
			"Type":                  aws.StringValue(ebsVolume.VolumeType),
			"IOPS":                  iops,
		}
		infoList = append(infoList, info)
	}
	return infoList, nil
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
		if strings.Contains(err.Error(), "InvalidVolume.NotFound"){
			//this volume does not exist, delete it from state if it exists
			util.ObjectDelete(volume)
			return nil, util.ErrorNotExistsInBackend()
		}
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

	if len(ebsVolume.Attachments) != 0 && aws.StringValue(ebsVolume.Attachments[0].Device) != ""{
		info["AWSMountPoint"] = aws.StringValue(ebsVolume.Attachments[0].Device)
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
	if len(volumeIDs) == 0{
		return make(map[string]map[string]string), nil
	}
	volumeInfos, err  := d.GetVolumesInfo(volumeIDs)
	if err != nil {
		return nil, err
	}
	for i, vol := range volumeInfos {
		volumes[vol[OPT_VOLUME_NAME]] = volumeInfos[i]
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
