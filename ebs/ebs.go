package ebs

import (
	"errors"
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
	EBS_CLUSTER_NAME        = "ebs.clustername"
	EBS_DEFAULT_VOLUME_KEY  = "ebs.defaultkmskeyid"
	EBS_DEFAULT_ENCRYPTED   = "ebs.defaultencrypted"
	EBS_DEFAULT_FILESYSTEM  = "ebs.defaultfilesystem"
	EBS_AUTOFORMAT          = "ebs.autoformat"
	EBS_AUTORESIZEFS        = "ebs.autoresizefs"

	DEFAULT_VOLUME_SIZE  = "4G"
	DEFAULT_VOLUME_TYPE  = "gp2"
	DEFAULT_CLUSTER_NAME = ""
	DEFAULT_FILESYSTEM   = "ext4"

	MOUNTS_DIR    = "mounts"
	MOUNT_BINARY  = "mount"
	UMOUNT_BINARY = "umount"
)

type Driver struct {
	mutex      *sync.RWMutex
	ebsService EBS
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
		return "", errors.New("BUG: Invalid empty device config path")
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

func shouldFailover(tags []*ec2.Tag) bool {
	if strings.ToLower(getTagValue("Failover", tags)) == "false" {
		return false
	}
	return true
}

func (d *Driver) blankVolume(name string) *Volume {
	return &Volume{
		configPath: d.Root,
		Name:       name,
	}
}

func (v *Volume) ConfigFile() (string, error) {
	if v.Name == "" {
		return "", errors.New("BUG: Invalid empty volume name")
	}
	if v.configPath == "" {
		return "", errors.New("BUG: Invalid empty volume config path")
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
	if err := Register(DRIVER_NAME, Init); err != nil {
		panic(err)
	}
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
		return fmt.Errorf("Invalid volume type=%v", volumeType)
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

func getDefaultDevice(root string, config map[string]string) (*Device, error) {
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
	log.Debugf("Setting driver DCName=%q", config[EBS_CLUSTER_NAME])
	dcName := config[EBS_CLUSTER_NAME]
	if config[EBS_DEFAULT_FILESYSTEM] == "" {
		config[EBS_DEFAULT_FILESYSTEM] = DEFAULT_FILESYSTEM
	} else {
		log.Debugf("Setting driver default filesystem type=%q", config[EBS_DEFAULT_FILESYSTEM])
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
	if autoFormatStr, ok := config[EBS_AUTOFORMAT]; ok {
		if autoFormat, err = strconv.ParseBool(autoFormatStr); err != nil {
			return nil, err
		}
	}
	if autoResizeStr, ok := config[EBS_AUTORESIZEFS]; ok {
		if autoResizefs, err = strconv.ParseBool(autoResizeStr); err != nil {
			return nil, err
		}
	}
	log.Debugf("Setting driver flags for autoFormat=%v autoResizefs=%v", autoFormat, autoResizefs)

	dev := &Device{
		Root:              root,
		DefaultVolumeSize: size,
		DefaultVolumeType: volumeType,
		DefaultDCName:     dcName,
		DefaultFSType:     fsType,
		DefaultKmsKeyID:   kmsKeyId,
		DefaultEncrypted:  encrypted,
		AutoFormat:        autoFormat,
		AutoResizeFS:      autoResizefs,
	}
	return dev, nil
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

		dev, err := getDefaultDevice(root, config)
		if err != nil {
			return nil, err
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
	infos["InstanceID"] = d.ebsService.GetInstanceID()
	infos["Region"] = d.ebsService.GetRegion()
	infos["AvailiablityZone"] = d.ebsService.GetAvailabilityZone()
	infos["AutoResizeFS"] = fmt.Sprint(d.AutoResizeFS)
	infos["AutoFormat"] = fmt.Sprint(d.AutoFormat)
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
		return "", 0, errors.New("Invalid IOPS for volume type io1")
	}
	if volumeType != "io1" && iops != 0 {
		return "", 0, errors.New("IOPS only valid for volume type io1")
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

func (d *Driver) MarkVolumeForGC(volumeId string) error {
	if volumeId == "" {
		return nil
	}
	gcTag := make(map[string]string)
	gcTag["GarbageCollection"] = time.Now().String()
	if err := d.ebsService.AddTags(volumeId, gcTag); err != nil {
		return err
	}
	return nil
}

func (d *Driver) CreateAndBuildFromSnapshot(volume *ec2.Volume, args *BuildArgs) (*BuildReturn, error) {
	// Create snapshot from volume in other AZ
	log.Debugf("Creating new snapshot from volumeId=%v", *volume.VolumeId)
	snapshotId, err := d.ebsService.LaunchSnapshot(*volume.VolumeId, fmt.Sprintf("Convoy: Creating snapshot from volume=%v", *volume.VolumeId), convertEc2TagsToMap(volume.Tags))
	if err != nil {
		return nil, err
	}
	log.Debugf("SnapshotId=%v was successfully created from volumeId=%v", snapshotId, *volume.VolumeId)
	snapshot, err := d.ebsService.GetSnapshot(snapshotId)
	if err != nil {
		return nil, err
	}
	return d.BuildFromSnapshot(snapshot, args)
}

func (d *Driver) BuildFromSnapshot(snapshot *ec2.Snapshot, args *BuildArgs) (*BuildReturn, error) {
	if err := d.ebsService.WaitForSnapshotComplete(*snapshot.SnapshotId); err != nil {
		return nil, err
	}
	log.Debugf("Snapshot=%v is ready", *snapshot.SnapshotId)
	snapshotVolumeSize := *snapshot.VolumeSize * GB
	volumeSize, err := d.getSize(args.opts, snapshotVolumeSize)
	if err != nil {
		return nil, err
	}
	if volumeSize < snapshotVolumeSize {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("Volume size cannot be less than snapshot size=%v", snapshotVolumeSize), util.ErrInvalidRequestCode)
	}

	volumeType, iops, err := d.getTypeAndIOPS(args.opts)
	if err != nil {
		return nil, err
	}

	r := &CreateEBSVolumeRequest{
		Size:       volumeSize,
		SnapshotID: *snapshot.SnapshotId,
		VolumeType: volumeType,
		IOPS:       iops,
		Tags:       convertEc2TagsToMap(snapshot.Tags),
		Encrypted:  *snapshot.Encrypted,
	}
	log.Debugf("Creating new volume from snapshotId=%v ", *snapshot.SnapshotId)
	volumeID, err := d.ebsService.CreateVolume(r)
	if err != nil {
		return nil, err
	}
	log.Debugf("Created volume=%v from EBS snapshot=%v", volumeID, *snapshot.SnapshotId)
	// If there is an old volume then we will mark is for GarbageCollection
	if err := d.MarkVolumeForGC(args.volumeId); err != nil {
		return nil, err
	}
	return &BuildReturn {
		volumeId: volumeID,
		volumeSize: volumeSize,
	}, nil
}

func (d *Driver) UpdateTags(volumeID string, newTags map[string]string) error {
	if err := d.ebsService.AddTags(volumeID, newTags); err != nil {
		log.Errorf("Failed to update tags for volume=%v: %s", volumeID, err)
		return err
	}
	return nil
}

func (d *Driver) BuildNewVolume(args *BuildArgs) (*BuildReturn, error) {
	volumeSize, err := d.getSize(args.opts, d.DefaultVolumeSize)
	if err != nil {
		return nil, err
	}
	volumeType, iops, err := d.getTypeAndIOPS(args.opts)
	if err != nil {
		return nil, err
	}
	r := &CreateEBSVolumeRequest{
		Size:       volumeSize,
		VolumeType: volumeType,
		IOPS:       iops,
		KmsKeyID:   d.DefaultKmsKeyID,
		Encrypted:  d.DefaultEncrypted,
	}
	volumeID, err := d.ebsService.CreateVolume(r)
	if err != nil {
		return nil, err
	}
	log.Debugf("Created new volume name=%v from EBS volume=%v", args.volumeName, volumeID)
	return &BuildReturn{
		volumeId: volumeID, 
		volumeSize: volumeSize,
	}, nil
}

func (d *Driver) BuildVolumeFromScratch(volume *ec2.Volume, snapshot *ec2.Snapshot, args *BuildArgs) (*BuildReturn, error) {
	// If there is no current reference to the volumeName or the snapshot has opted out of failover the build a volume from scratch
	if snapshot == nil && volume == nil {
		log.Debugf("No recent snapshot or recent volume - Building name=%v from scratch", args.volumeName)
		buildReturn, err := d.BuildNewVolume(args)
		return buildReturn, err
	} else if snapshot != nil && !shouldFailover(snapshot.Tags) {
		// If there is a snapshot, but it opted out of failover then build from scratch
		log.Debugf("A recent snapshot=%v exists for name=%v, but it has opted out of failover", *snapshot.SnapshotId, args.volumeName)
		buildReturn, err := d.BuildNewVolume(args)
		return buildReturn, err
	} else if volume != nil && !d.isVolumeInLocalAz(volume) && !shouldFailover(volume.Tags) {
		log.Debugf("A recent volume=%v exists for name=%v in AZ=%v, but it has opted out of failover", *volume.VolumeId, args.volumeName, *volume.AvailabilityZone)
		buildReturn, err := d.BuildNewVolume(args)
		return buildReturn, err
	}
	// The other scenarios where a volume exists would never trigger a build from scratch
	// If a snapshot exists and it doesn't opt-out of failover then it would run BuildFromSnapshot
	return nil, nil
}

func (d *Driver) isVolumeInLocalAz(volume *ec2.Volume) bool {
	if volume == nil {
		return false
	}
	return *volume.AvailabilityZone == d.ebsService.GetAvailabilityZone()
}

func (d *Driver) MountLocalVolume(volume *ec2.Volume, snapshot *ec2.Snapshot, args *BuildArgs) (*BuildReturn, error) {
	// Nothing we can do if a most recent volume does not exist
	if volume != nil && d.isVolumeInLocalAz(volume) {
		// If the most recent volume is in the current availability zone, then the only time we would not mount
		// is if the snapshot is of a different volume and is more up to date
		if snapshot == nil || (*snapshot.VolumeId) == (*volume.VolumeId) || (*snapshot.StartTime).Before(*volume.CreateTime) {
			log.Debugf("Taking MountLocalVolume path for volumeId=%v", *volume.VolumeId)
			return &BuildReturn {
				volumeId: *volume.VolumeId,
				volumeSize: *volume.Size * GB,
			}, nil
		}
	}
	return nil, nil
}

func (d *Driver) RebuildFromRemoteVolume(volume *ec2.Volume, snapshot *ec2.Snapshot, args *BuildArgs) (*BuildReturn, error) {
	// If a most recent volume exists and it's in another AZ the rebuild from it
	if volume != nil && !d.isVolumeInLocalAz(volume) && shouldFailover(volume.Tags) {
		log.Debugf("Taking RebuildFromRemoteVolume path for name=%v, volumeId=%v", args.volumeName, *volume.VolumeId)
		buildReturn, err := d.CreateAndBuildFromSnapshot(volume, args)
		return buildReturn, err
	}
	return nil, nil
}

func (d *Driver) RebuildFromSnapshot(volume *ec2.Volume, snapshot *ec2.Snapshot, args *BuildArgs) (*BuildReturn, error) {
	if snapshot != nil && shouldFailover(snapshot.Tags) {
		// There is a snapshot but no volume then build from snapshot
		if volume == nil {
			log.Debugf("Taking RebuildFromSnapshot path for volume name=%v and snapshotId=%v", args.volumeName, *snapshot.SnapshotId)
			return d.BuildFromSnapshot(snapshot, args)
		} else if d.isVolumeInLocalAz(volume) && (*snapshot.VolumeId) != (*volume.VolumeId) && (*snapshot.StartTime).After(*volume.CreateTime) {
			// If the most recent volume is local AND the snapshot is not of the local volume AND the snapshot is newer
			log.Debugf("Taking RebuildFromSnapshot path for volume name=%v and snapshotId=%v", args.volumeName, *volume.VolumeId)
			return d.BuildFromSnapshot(snapshot, args)
		}
	}
	return nil, nil
}

type BuildArgs struct {
	volumeName string
	volumeId   string
	opts       map[string]string
	tags       map[string]string
}

type BuildReturn struct {
	volumeId   string
	volumeSize int64
}

func (d *Driver) BuildVolume(volumeName string, volumeID string, opts map[string]string, newTags map[string]string) (*BuildReturn, error) {
	liveAvailabilityZones, err := d.ebsService.GetAvailabilityZones(
		&ec2.Filter{
			Name: aws.String("state"),
			Values: []*string{
				aws.String("available"),
			},
		},
	)
	if err != nil {
		return nil, err
	} else if len(liveAvailabilityZones) == 0 {
		return nil, util.NewConvoyDriverErr(errors.New("AWS is reporting not any Availablity Zones in \"available\" state"), util.ErrGenericFailureCode)
	}

	// Need to specify the volume and the filter for availability-zones
	mostRecentVolume, err := d.ebsService.GetMostRecentAvailableVolume(
		volumeName,
		d.DefaultDCName,
		&ec2.Filter{
			Name:   aws.String("availability-zone"),
			Values: liveAvailabilityZones,
		},
	)
	if err != nil {
		return nil, err
	}

	mostRecentSnapshot, err := d.ebsService.GetMostRecentSnapshot(volumeName, d.DefaultDCName)
	if err != nil {
		return nil, err
	}

	// if volumeID is empty and most recent volume is empty then blow up
	if volumeID != "" && mostRecentVolume == nil {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("No available volume for volume=%v", volumeID), util.ErrVolumeNotAvailableCode)
	}

	var oldVolume *ec2.Volume
	if volumeID != "" {
		if oldVolume, err = d.ebsService.GetVolume(volumeID); err != nil {
			return nil, err
		}
		// If Failover is False, then return from failover logic
		if !shouldFailover(oldVolume.Tags) {
			return &BuildReturn {
				volumeId: *oldVolume.VolumeId, 
				volumeSize: *oldVolume.Size * GB,
			}, nil
		}
	}

	args := &BuildArgs{
		volumeName: volumeName,
		volumeId:   volumeID,
		opts:       opts,
		tags:       newTags,
	}

	// Debug statements only
	if mostRecentVolume != nil {
		log.Debugf("MostRecentVolume was found for (name=%v, volumeId=%v)", volumeName, *mostRecentVolume.VolumeId) 
	} else {
		log.Debugf("MostRecentVolume is nil for (name=%v)", volumeName)
	}
	if mostRecentSnapshot != nil {
		log.Debugf("MostRecentSnapshot was found for (name=%v, volumeId=%v)", volumeName, *mostRecentSnapshot.SnapshotId) 
	} else {
		log.Debugf("MostRecentSnapshot is nil for (name=%v)", volumeName)
	}

	buildReturn, err := d.BuildVolumeFromScratch(mostRecentVolume, mostRecentSnapshot, args)
	if err != nil {
		return nil, err
	} else if buildReturn != nil {
		return buildReturn, nil
	}
	buildReturn, err = d.MountLocalVolume(mostRecentVolume, mostRecentSnapshot, args)
	if err != nil {
		return nil, err
	} else if buildReturn != nil {
		return buildReturn, nil
	}

	buildReturn, err = d.RebuildFromRemoteVolume(mostRecentVolume, mostRecentSnapshot, args)
	if err != nil {
		return nil, err
	} else if buildReturn != nil {
		return buildReturn, nil
	}

	buildReturn, err= d.RebuildFromSnapshot(mostRecentVolume, mostRecentSnapshot, args)
	if err != nil {
		return nil, err
	} else if buildReturn != nil {
		return buildReturn, nil
	}
	return nil, fmt.Errorf("This scenario was not captured by any failover logic. Volume=%+v. Snapshot=%+v", mostRecentVolume, mostRecentSnapshot)
}

func (d *Driver) CreateVolume(req Request) error {
	var (
		err        error
		volumeSize int64
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
		return util.NewConvoyDriverErr(fmt.Errorf("Volume=%v already exists", id), util.ErrVolumeExistsCode)
	}

	//check if the volume name is a 64 chars alphanumeric string.
	//Since this might imply a random name generated by Docker.
	//This check prevents run-away volume creation in EBS
	re := regexp.MustCompile("^[a-zA-Z0-9]{64}$")
	if re.MatchString(id) {
		return util.NewConvoyDriverErr(errors.New("Volume name 64 chars alphanumeric. Are you sure this is not a Docker generated volume name? Please change the name and try again."), util.ErrInvalidRequestCode)
	}

	//EBS volume name
	volumeName := opts[OPT_VOLUME_NAME]
	//EBS volume ID
	volumeID := opts[OPT_VOLUME_DRIVER_ID]
	backupURL := opts[OPT_BACKUP_URL]
	if backupURL != "" && volumeID != "" {
		return util.NewConvoyDriverErr(errors.New("Cannot specify both backup and EBS volume ID"), util.ErrInvalidRequestCode)
	}
	if volumeID != "" && volumeName != "" {
		return util.NewConvoyDriverErr(errors.New("Cannot specify both EBS volume ID and EBS volume Name"), util.ErrInvalidRequestCode)
	}
	if volumeName != "" {
		log.Debugf("Checking if volume with name=%v is in EBS", volumeName)
		ebsVolume, err := d.ebsService.GetVolumeByName(volumeName, d.DefaultDCName)
		if err != nil {
			log.Debugf("GetVolumeByName error for name=%v: %+v", volumeName, err)
			if convoyErr, ok := err.(*util.ConvoyDriverErr); ok {
				log.Debugf("GetVolumeByName for name=%v produced a convoy error: %+v", volumeName, convoyErr)
				if convoyErr.ErrorCode != util.ErrVolumeNotFoundCode {
					return util.NewConvoyDriverErr(fmt.Errorf("Got an unexpected error when looking up name=%v: %s", volumeName, convoyErr), util.ErrGenericFailureCode)
				}
			}
		} else {
			volumeSize = *ebsVolume.Size * GB
			volumeID = aws.StringValue(ebsVolume.VolumeId)
			log.Debugf("Received EBS volume=%v with name=%v", volumeID, volumeName)
			//is size of the existing block is same as that requested
			requestedSize, _ := util.ParseSize(opts[OPT_SIZE])
			if requestedSize > 0 && volumeSize != requestedSize {
				log.Debugf("Volume size requested (%d GB) does not match actual volume size (%d GB)", requestedSize/GB, volumeSize/GB)
				return util.NewConvoyDriverErr(fmt.Errorf("Volume size requested (%d GB) does not match actual volume size (%d GB)", requestedSize/GB, volumeSize/GB), util.ErrInvalidRequestCode)
			}
		}
	}

	newTags := map[string]string{
		"Name":   id,
		"DCName": d.DefaultDCName,
	}

	// If Failover Tag is false, will be designated inside this logic and return the proper values
	buildReturn, err := d.BuildVolume(volumeName, volumeID, opts, newTags)
	if err != nil {
		return err
	}
	volumeID = buildReturn.volumeId
	volumeSize = buildReturn.volumeSize
	if err := d.UpdateTags(volumeID, newTags); err != nil {
		return err
	}

	dev, err := d.ebsService.AttachVolume(volumeID, volumeSize)
	if err != nil {
		return err
	}
	log.Debugf("Attached EBS volume=%v to dev=%v", volumeID, dev)

	volume.Name = id
	volume.EBSID = volumeID
	volume.Device = dev
	volume.Snapshots = make(map[string]Snapshot)

	var needsFS bool
	if fsType, err := fs.Detect(volume.Device); err != nil {
		if err == fs.ErrNoFilesystemDetected {
			needsFS = true
		} else {
			return err
		}
	} else {
		log.Debugf("Detected existing filesystem type=%v for device=%v", fsType, volume.Device)
		if d.AutoResizeFS {
			log.Debugf("Ensuring filesystem size and device=%v size match", volume.Device)
			if err := fs.Resize(volume.Device); err != nil {
				log.Debugf("Syncing device=%s sizes error: %s", volume.Device, err)
				return err
			}
		}
	}

	if needsFS && d.AutoFormat {
		log.Debugf("Formatting device=%v with filesystem type=%v", volume.Device, d.DefaultFSType)
		if err := fs.FormatDevice(volume.Device, d.DefaultFSType); err != nil {
			return err
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
		log.Warnf("Unable to detach id=%v/ebsid=%v due to backend error: %s - Deleting object from state", id, volume.EBSID, err)
	} else {
		log.Debugf("Successfully detached id=%v/ebsid=%v from dev=%v", id, volume.EBSID, volume.Device)
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
		// if device doesn't exist, it's a stale entry.
		errorStr := err.Error()
		var validID = regexp.MustCompile(`. output mount: special device /dev/([a-z]+) does not exist`)
		if validID.MatchString(errorStr) {
			// Delete from convoy's internal state.
			if delErr := util.ObjectDelete(volume); delErr != nil {
				log.Warnf("Deleting object=%v from internal state in response to an error while mounting produced another error: %s", volume, delErr)
			}
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

	// Medallia specific, unmount docker volume also implies a detach
	detachErr := d.DeleteVolume(req)
	if detachErr != nil {
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

	log.Debugf("Looking for IDs of names: %v", ids)

	var ebsIDs []string
	var volumeObjects []*Volume
	for _, id := range ids {
		volume := d.blankVolume(id)
		if err := util.ObjectLoad(volume); err != nil {
			return nil, err
		}
		ebsIDs = append(ebsIDs, volume.EBSID)
		volumeObjects = append(volumeObjects, volume)
	}

	log.Debugf("Found EBS IDs: %v", ebsIDs)

	var wg sync.WaitGroup
	ebsVolumes := make([]*ec2.Volume, len(ebsIDs))

	for k, ebsID := range ebsIDs {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			if ebsVolume, err := d.ebsService.GetVolume(id); err != nil {
				if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
					log.Debugf("Found a non-existent volume=%v in state, deleting", ids[i])
					vol := d.blankVolume(ids[i])
					if loadErr := util.ObjectLoad(vol); loadErr != nil {
						log.Warnf("Problem loading volume=%v: %s - Continuing despite this error", vol, loadErr)
					}
					if delErr := util.ObjectDelete(vol); delErr != nil {
						log.Warnf("Problem deleting volume=%v: %s - Continuing despite this error", vol, delErr)
					}
				}
			} else {
				ebsVolumes[i] = ebsVolume
			}
		}(k, ebsID)
	}
	wg.Wait()

	var infoList []map[string]string
	for i, ebsVolume := range ebsVolumes {
		if ebsVolume == nil {
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
		if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
			//this volume does not exist, delete it from state if it exists
			if delErr := util.ObjectDelete(volume); delErr != nil {
				log.Warnf("Problem deleting volume=%v: %s - Continuing despite this error", volume, delErr)
			}
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

	if len(ebsVolume.Attachments) != 0 && aws.StringValue(ebsVolume.Attachments[0].Device) != "" {
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
	if len(volumeIDs) == 0 {
		return make(map[string]map[string]string), nil
	}
	volumeInfos, err := d.GetVolumesInfo(volumeIDs)
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
	ebsSnapshotID, err := d.ebsService.LaunchSnapshot(volume.EBSID, "Convoy Snapshot", tags)
	if err != nil {
		return err
	}

	log.Debugf("Creating snapshot id=%v/ebsid=%v of volume=%v/ebsid=%v", id, ebsSnapshotID, volumeID, volume.EBSID)

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

	log.Debugf("Removing reference of snapshot id=%v/ebsid=%v of volume=%v/ebsid=%v", id, snapshot.EBSID, volumeID, volume.EBSID)
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
		return fmt.Errorf("invalid EBS snapshot id=%v", id)
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
		return "", "", fmt.Errorf("BUG: Why dispatch scheme=%v to driver=%v?", u.Scheme, DRIVER_NAME)
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
	return encodeURL(d.ebsService.GetRegion(), snapshot.EBSID), nil
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
