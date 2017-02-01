package ebs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/rancher/convoy/util"
)

const (
	GB             = 1073741824
	RETRY_INTERVAL = 5
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "ebs"})
)

type ebsService struct {
	metadataClient *ec2metadata.EC2Metadata
	ec2Client      *ec2.EC2

	InstanceID       string
	Region           string
	AvailabilityZone string
}

type CreateEBSVolumeRequest struct {
	Size       int64
	IOPS       int64
	SnapshotID string
	VolumeType string
	Tags       map[string]string
	KmsKeyID   string
	Encrypted  bool
}

type CreateSnapshotRequest struct {
	VolumeID    string
	Description string
	Tags        map[string]string
}

type VolumeByCreateTime []*ec2.Volume

func (v VolumeByCreateTime) Len() int {
	return len(v)
}
func (v VolumeByCreateTime) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}
func (v VolumeByCreateTime) Less(i, j int) bool {
	return (*v[i].CreateTime).Before(*v[j].CreateTime)
}

type SnapshotByTime []*ec2.Snapshot

func (s SnapshotByTime) Len() int {
	return len(s)
}
func (s SnapshotByTime) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s SnapshotByTime) Less(i, j int) bool {
	return (*s[i].StartTime).Before(*s[j].StartTime)
}
func sleepBeforeRetry() {
	time.Sleep(RETRY_INTERVAL * time.Second)
}

func parseAwsError(err error) error {
	if err == nil {
		return nil
	}
	if awsErr, ok := err.(awserr.Error); ok {
		message := fmt.Sprintln("AWS Error: ", awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			message += fmt.Sprintln(reqErr.StatusCode(), reqErr.RequestID())
		}
		return util.NewConvoyDriverErr(fmt.Errorf(message), util.ErrGenericFailureCode)
	}
	return util.NewConvoyDriverErr(err, util.ErrGenericFailureCode)
}

func NewEBSService() (*ebsService, error) {
	var err error

	s := &ebsService{}
	s.metadataClient = ec2metadata.New(session.New())
	if !s.isEC2Instance() {
		return nil, util.NewConvoyDriverErr(errors.New("Not running on an EC2 instance"), util.ErrInvalidRequestCode)
	}

	s.InstanceID, err = s.metadataClient.GetMetadata("instance-id")
	if err != nil {
		return nil, err
	}

	s.Region, err = s.metadataClient.Region()
	if err != nil {
		return nil, err
	}

	s.AvailabilityZone, err = s.metadataClient.GetMetadata("placement/availability-zone")
	if err != nil {
		return nil, err
	}

	config := aws.NewConfig().WithRegion(s.Region)
	s.ec2Client = ec2.New(session.New(), config)
	return s, nil
}

func (s *ebsService) isEC2Instance() bool {
	return s.metadataClient.Available()
}

func (s *ebsService) waitForVolumeTransition(volumeID, start, end string) error {
	log.Debugf("Starting wait from %s to %s for %s", start, end, volumeID)
	volume, err := s.GetVolume(volumeID)
	if err != nil {
		log.Errorf("Got error to get volume %s", volumeID)
		return err
	}

	timeChan := time.NewTimer(time.Second * 61).C
	tickChan := time.NewTicker(time.Second * 3).C //tick at most 20 times before err out

POLL:
	for *volume.State == start {
		select {
		case <-timeChan:
			log.Debugf("Timeout Reached, stopping to poll volume state")
			break POLL
		case <-tickChan:
			log.Debugf("Waiting for volume %v state transiting from %v to %v",
				volumeID, start, end)
			volume, err = s.GetVolume(volumeID)
			if err != nil {
				return err
			}
		}
	}
	if *volume.State != end {
		return util.NewConvoyDriverErr(fmt.Errorf("Cannot finish volume %v state transition, from %v to %v, though final state %v",
			volumeID, start, end, *volume.State), util.ErrVolumeTransitionCode)
	}
	return nil
}

func (s *ebsService) waitForVolumeAttaching(volumeID string) error {
	var attachment *ec2.VolumeAttachment
	volume, err := s.GetVolume(volumeID)
	if err != nil {
		return err
	}
	for len(volume.Attachments) == 0 {
		log.Debugf("Retry to get attachment of volume")
		volume, err = s.GetVolume(volumeID)
		if err != nil {
			return err
		}
	}
	attachment = volume.Attachments[0]

	timeChan := time.NewTimer(time.Second * 61).C
	tickChan := time.NewTicker(time.Second * 3).C //tick at most 20 times before err out

WAIT:
	for *attachment.State == ec2.VolumeAttachmentStateAttaching {
		select {
		case <-timeChan:
			log.Debugf("Timeout Reached, stopping to wait for volume to attach")
			break WAIT
		case <-tickChan:
			log.Debugf("Waiting for volume %v attaching", volumeID)
			volume, err := s.GetVolume(volumeID)
			if err != nil {
				return err
			}
			if len(volume.Attachments) != 0 {
				attachment = volume.Attachments[0]
			} else {
				return util.NewConvoyDriverErr(fmt.Errorf("Attaching failed for %s", volumeID), util.ErrVolumeAttachFailureCode)
			}
		}
	}
	if *attachment.State != ec2.VolumeAttachmentStateAttached {
		return util.NewConvoyDriverErr(fmt.Errorf("Cannot attach volume, final state %v", *attachment.State), util.ErrVolumeAttachFailureCode)
	}
	return nil
}

func (s *ebsService) GetAvailabilityZones(filters ...*ec2.Filter) ([]*string, error) {
	azInput := &ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("region-name"),
				Values: []*string{
					aws.String(s.Region),
				},
			},
		},
	}
	// Find out which other AZ's are available
	var liveAZs []*string
	azOutput, err := s.ec2Client.DescribeAvailabilityZones(azInput)
	if err != nil {
		return liveAZs, err
	}
	for _, az := range azOutput.AvailabilityZones {
		liveAZs = append(liveAZs, az.ZoneName)
	}
	return liveAZs, nil
}

func (s *ebsService) CreateVolume(request *CreateEBSVolumeRequest) (string, error) {
	if request == nil {
		return "", util.NewConvoyDriverErr(errors.New("Invalid CreateEBSVolumeRequest"), util.ErrInvalidRequestCode)
	}
	size := request.Size
	iops := request.IOPS
	snapshotID := request.SnapshotID
	volumeType := request.VolumeType
	kmsKeyID := request.KmsKeyID

	// EBS size are in GB, we would round it up
	ebsSize := size / GB
	if size%GB > 0 {
		ebsSize += 1
	}

	params := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(s.AvailabilityZone),
		Size:             aws.Int64(ebsSize),
		Encrypted:        aws.Bool(request.Encrypted),
	}

	if snapshotID != "" {
		params.SnapshotId = aws.String(snapshotID)
	} else if kmsKeyID != "" {
		params.KmsKeyId = aws.String(kmsKeyID)
		params.Encrypted = aws.Bool(true)
	}

	if volumeType != "" {
		if err := checkVolumeType(volumeType); err != nil {
			return "", util.NewConvoyDriverErr(err, util.ErrInvalidRequestCode)
		}
		if volumeType == "io1" && iops == 0 {
			return "", util.NewConvoyDriverErr(errors.New("Invalid IOPS for volume type io1"), util.ErrInvalidRequestCode)
		}
		if volumeType != "io1" && iops != 0 {
			return "", util.NewConvoyDriverErr(errors.New("IOPS only valid for volume type io1"), util.ErrInvalidRequestCode)
		}
		params.VolumeType = aws.String(volumeType)
		if iops != 0 {
			params.Iops = aws.Int64(iops)
		}
	}

	ec2Volume, err := s.ec2Client.CreateVolume(params)
	if err != nil {
		return "", parseAwsError(err)
	}

	volumeID := *ec2Volume.VolumeId
	if err = s.waitForVolumeTransition(volumeID, ec2.VolumeStateCreating, ec2.VolumeStateAvailable); err != nil {
		log.Debug("Failed to create volume: ", err)
		err = s.DeleteVolume(volumeID)
		if err != nil {
			log.Errorf("Failed deleting volume: %v", parseAwsError(err))
		}
		return "", util.NewConvoyDriverErr(fmt.Errorf("Failed creating volume with size %v and snapshot %v",
			size, snapshotID), util.ErrVolumeCreateFailureCode)
	}
	if request.Tags != nil {
		if err := s.AddTags(volumeID, request.Tags); err != nil {
			log.Warnf("Unable to tag %v with %v, but continue", volumeID, request.Tags)
		}
	}

	return volumeID, nil
}

func (s *ebsService) DeleteVolume(volumeID string) error {
	params := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volumeID),
	}
	_, err := s.ec2Client.DeleteVolume(params)
	return parseAwsError(err)
}

func (s *ebsService) GetVolumes(volumeIDs []string) ([]*ec2.Volume, error) {
	sleepBeforeRetry()
	var idList []*string
	params := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("availability-zone"),
				Values: []*string{
					aws.String(s.AvailabilityZone),
				},
			},
		},
	}

	for _, volumeID := range volumeIDs {
		idList = append(idList, aws.String(volumeID))
	}

	params.VolumeIds = idList

	volumes, err := s.ec2Client.DescribeVolumes(params)
	if err != nil {
		return nil, parseAwsError(err)
	}
	if len(volumes.Volumes) < 1 {
		log.Errorf("Cannot find any volumes from the list provided: %v", volumeIDs)
		return nil, util.NewConvoyDriverErr(errors.New("Cannot find any volumes"), util.ErrVolumeNotFoundCode)
	}
	return volumes.Volumes, nil
}

func (s *ebsService) GetVolume(volumeID string) (*ec2.Volume, error) {
	sleepBeforeRetry()
	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			aws.String(volumeID),
		},
		Filters: []*ec2.Filter{
			{
				Name: aws.String("availability-zone"),
				Values: []*string{
					aws.String(s.AvailabilityZone),
				},
			},
		},
	}
	volumes, err := s.ec2Client.DescribeVolumes(params)
	if err != nil {
		return nil, parseAwsError(err)
	}
	if len(volumes.Volumes) != 1 {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("Cannot find volume %v", volumeID), util.ErrVolumeNotFoundCode)
	}
	return volumes.Volumes[0], nil
}

func (s *ebsService) GetVolumeByName(volumeName, dcName string) (*ec2.Volume, error) {
	params := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(volumeName),
				},
			},
			{
				Name: aws.String("tag:DCName"),
				Values: []*string{
					aws.String(dcName),
				},
			},
			{
				Name: aws.String("availability-zone"),
				Values: []*string{
					aws.String(s.AvailabilityZone),
				},
			},
		},
	}
	volumes, err := s.ec2Client.DescribeVolumes(params)
	if err != nil {
		return nil, parseAwsError(err)
	}

	var finalVolumes []*ec2.Volume
	for _, volume := range volumes.Volumes {
		if getTagValue("GarbageCollection", volume.Tags) == "" {
			finalVolumes = append(finalVolumes, volume)
		}
	}
	// Since tag Name is not AWS's identifying attribute (i.e. volume_id), we can get multiple results with same name
	// Return the last one, i.e. the latest one.
	if len(finalVolumes) == 0 {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("Cannot find volume by name %s in region %s in az %s", volumeName, s.Region, s.AvailabilityZone), util.ErrVolumeNotFoundCode)
	} else if len(finalVolumes) > 1 {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("Found multiple volumes with name %s", volumeName), util.ErrVolumeMultipleInstancesCode)
	}
	return finalVolumes[0], nil
}

func getBlkDevList() (map[string]bool, error) {
	devList := make(map[string]bool)
	dirList, err := ioutil.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	for _, dir := range dirList {
		devList[dir.Name()] = true
	}
	return devList, nil
}

func getAttachedDev(oldDevList map[string]bool, size int64) (string, error) {
	newDevList, err := getBlkDevList()
	attachedDev := ""
	if err != nil {
		return "", err
	}
	for dev := range newDevList {
		if oldDevList[dev] {
			continue
		}
		devSizeInSectorStr, err := ioutil.ReadFile("/sys/block/" + dev + "/size")
		if err != nil {
			return "", err
		}
		devSize, err := strconv.ParseInt(strings.TrimSpace(string(devSizeInSectorStr)), 10, 64)
		if err != nil {
			return "", err
		}
		devSize *= 512
		if devSize == size {
			if attachedDev != "" {
				return "", fmt.Errorf("Found more than one device matching description, %v and %v",
					attachedDev, dev)
			}
			attachedDev = dev
		}
	}
	if attachedDev == "" {
		return "", util.NewConvoyDriverErr(errors.New("Cannot find a device matching description"), util.ErrDeviceFailureCode)
	}
	return "/dev/" + attachedDev, nil
}

func (s *ebsService) getInstanceDevList() (map[string]bool, error) {
	params := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("attachment.instance-id"),
				Values: []*string{
					aws.String(s.InstanceID),
				},
			},
		},
	}
	volumes, err := s.ec2Client.DescribeVolumes(params)
	if err != nil {
		return nil, parseAwsError(err)
	}
	devMap := make(map[string]bool)
	for _, volume := range volumes.Volumes {
		if len(volume.Attachments) == 0 {
			continue
		}
		devMap[*volume.Attachments[0].Device] = true
	}
	return devMap, nil
}

func (s *ebsService) FindFreeDeviceForAttach() (string, error) {
	availableDevs := make(map[string]bool)
	// Recommended available devices for EBS volume from AWS website
	chars := "fghijklmnop"
	for i := 0; i < len(chars); i++ {
		availableDevs["/dev/sd"+string(chars[i])] = true
	}
	devMap, err := s.getInstanceDevList()
	if err != nil {
		return "", err
	}
	for d := range devMap {
		if _, ok := availableDevs[d]; !ok {
			continue
		}
		availableDevs[d] = false
	}
	for dev, available := range availableDevs {
		if available {
			return dev, nil
		}
	}
	return "", util.NewConvoyDriverErr(fmt.Errorf("Cannot find an available device for instance %v", s.InstanceID), util.ErrDeviceFailureCode)
}

func (s *ebsService) AttachVolume(volumeID string, size int64) (string, error) {
	dev, err := s.FindFreeDeviceForAttach()
	if err != nil {
		return "", err
	}

	log.Debugf("Attaching %v to %v's %v", volumeID, s.InstanceID, dev)
	params := &ec2.AttachVolumeInput{
		Device:     aws.String(dev),
		InstanceId: aws.String(s.InstanceID),
		VolumeId:   aws.String(volumeID),
	}

	blkList, err := getBlkDevList()
	if err != nil {
		return "", err
	}

	if _, err := s.ec2Client.AttachVolume(params); err != nil {
		return "", parseAwsError(err)
	}

	if err = s.waitForVolumeAttaching(volumeID); err != nil {
		log.Errorf("Error in attaching: %s - Trying force detach to free the volume and returning err", err.Error())
		forceDetachParams := &ec2.DetachVolumeInput{
			VolumeId:   aws.String(volumeID),
			InstanceId: aws.String(s.InstanceID),
			Force:      aws.Bool(true),
		}

		if _, err := s.ec2Client.DetachVolume(forceDetachParams); err != nil {
			return "", parseAwsError(err)
		}

		forceDetachErr := s.waitForVolumeTransition(volumeID, ec2.VolumeAttachmentStateAttaching, ec2.VolumeStateAvailable)

		if forceDetachErr != nil {
			log.Errorf("Error in force detach's state transition: %s - Returning the error", forceDetachErr.Error())
			return "", util.NewConvoyDriverErr(fmt.Errorf("Force Detach Err: %s", forceDetachErr.Error()), util.ErrVolumeDetachFailureCode)
		}

		fdTag := make(map[string]string)
		fdTag["ForceDetached"] = "true"
		if tagErr := s.AddTags(s.InstanceID, fdTag); tagErr != nil {
			log.Warnf("Problem adding force-detach tags=%+v to instanceID=%q: %s (continuing despite this error)", fdTag, s.InstanceID, tagErr)
		}

		log.Debugf("Successfully force detached the volume %s", volumeID)
		return "", err
	}

	result, err := getAttachedDev(blkList, size)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (s *ebsService) DetachVolume(volumeID string) error {
	params := &ec2.DetachVolumeInput{
		VolumeId:   aws.String(volumeID),
		InstanceId: aws.String(s.InstanceID),
	}

	if _, err := s.ec2Client.DetachVolume(params); err != nil {
		return parseAwsError(err)
	}

	detachErr := s.waitForVolumeTransition(volumeID, ec2.VolumeStateInUse, ec2.VolumeStateAvailable)

	if detachErr != nil {
		//error in state transition, force detach and free volume
		log.Errorf("Error in detach's state transition: %s - Trying force detach to free the volume", detachErr.Error())
		forceDetachParams := &ec2.DetachVolumeInput{
			VolumeId:   aws.String(volumeID),
			InstanceId: aws.String(s.InstanceID),
			Force:      aws.Bool(true),
		}

		if _, err := s.ec2Client.DetachVolume(forceDetachParams); err != nil {
			return parseAwsError(err)
		}

		forceDetachErr := s.waitForVolumeTransition(volumeID, ec2.VolumeStateInUse, ec2.VolumeStateAvailable)

		if forceDetachErr != nil {
			log.Errorf("Error in force detach's state transition: %s - Returning the error", forceDetachErr.Error())
			return util.NewConvoyDriverErr(fmt.Errorf("Force Detach Err: %s", forceDetachErr.Error()), util.ErrVolumeDetachFailureCode)
		}

		fdTag := make(map[string]string)
		fdTag["ForceDetached"] = "true"
		if tagErr := s.AddTags(s.InstanceID, fdTag); tagErr != nil {
			log.Warnf("Problem adding force-detach tags=%+v to instanceID=%q: %s (continuing despite this error)", fdTag, s.InstanceID, tagErr)
		}

		log.Debugf("Successfully force detached the volume %s", volumeID)
		return forceDetachErr
	}
	log.Debugf("Successfully detached the volume %s", volumeID)
	return detachErr
}

func (s *ebsService) GetMostRecentSnapshot(volumeName string, dcName string, filters ...*ec2.Filter) (*ec2.Snapshot, error) {
	snapshots, err := s.GetSnapshots(volumeName, dcName, filters...)
	if err != nil {
		return nil, util.NewConvoyDriverErr(err, util.ErrSnapshotNotFoundCode)
	} else if len(snapshots) == 0 {
		return nil, nil
	}
	return snapshots[len(snapshots)-1], nil
}

func (s *ebsService) GetMostRecentVolume(volumeName string, dcName string, filters ...*ec2.Filter) (*ec2.Volume, error) {
	// We always need to specify the name, dc name, and that the snapshot is complete. If the AZ has truly crashed, then the snapshot may never complete
	volumeInput := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(volumeName),
				},
			},
			&ec2.Filter{
				Name: aws.String("tag:DCName"),
				Values: []*string{
					aws.String(dcName),
				},
			},
			&ec2.Filter{
				Name: aws.String("status"),
				Values: []*string{
					aws.String("available"),
				},
			},
		},
	}
	volumeInput.Filters = append(volumeInput.Filters, filters...)
	log.Printf("Describe volumes input: %+v", volumeInput)
	req, volOutput := s.ec2Client.DescribeVolumesRequest(volumeInput)
	if err := req.Send(); err != nil {
		return nil, util.NewConvoyDriverErr(err, util.ErrVolumeNotAvailableCode)
	}
	sort.Sort(sort.Reverse(VolumeByCreateTime(volOutput.Volumes)))
	if len(volOutput.Volumes) == 0 {
		return nil, nil
	}
	return volOutput.Volumes[0], nil
}

func (s *ebsService) LaunchSnapshot(volumeId string, description string, tags map[string]string) (string, error) {
	request := &CreateSnapshotRequest{
		VolumeID:    volumeId,
		Description: description,
		Tags:        tags,
	}
	return s.CreateSnapshot(request)
}

// Gets the snapshots. The name and dc name are required, but any extra filters are not
func (s *ebsService) GetSnapshots(volumeName string, dcName string, filters ...*ec2.Filter) ([]*ec2.Snapshot, error) {
	// We always need to specify the name, dc name, and that the snapshot is complete. If the AZ has truly crashed, then the snapshot may never complete
	snapshotInput := &ec2.DescribeSnapshotsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(volumeName),
				},
			},
			&ec2.Filter{
				Name: aws.String("tag:DCName"),
				Values: []*string{
					aws.String(dcName),
				},
			},
			&ec2.Filter{
				Name: aws.String("status"),
				Values: []*string{
					aws.String("completed"),
				},
			},
		},
	}
	snapshotInput.Filters = append(snapshotInput.Filters, filters...)
	req, snapOutput := s.ec2Client.DescribeSnapshotsRequest(snapshotInput)
	if err := req.Send(); err != nil {
		return []*ec2.Snapshot{}, err
	}
	sort.Sort(SnapshotByTime(snapOutput.Snapshots))
	return snapOutput.Snapshots, nil
}

func (s *ebsService) GetSnapshotWithRegion(snapshotID, region string) (*ec2.Snapshot, error) {
	params := &ec2.DescribeSnapshotsInput{
		SnapshotIds: []*string{
			aws.String(snapshotID),
		},
	}
	ec2Client := s.ec2Client
	if region != s.Region {
		ec2Client = ec2.New(session.New(), aws.NewConfig().WithRegion(region))
	}
	snapshots, err := ec2Client.DescribeSnapshots(params)
	if err != nil {
		return nil, parseAwsError(err)
	}
	if len(snapshots.Snapshots) != 1 {
		return nil, util.NewConvoyDriverErr(fmt.Errorf("Cannot find snapshot %v", snapshotID), util.ErrSnapshotNotFoundCode)
	}
	return snapshots.Snapshots[0], nil
}

func (s *ebsService) GetSnapshot(snapshotID string) (*ec2.Snapshot, error) {
	sleepBeforeRetry()
	return s.GetSnapshotWithRegion(snapshotID, s.Region)
}

func (s *ebsService) WaitForSnapshotComplete(snapshotID string) error {
	snapshot, err := s.GetSnapshot(snapshotID)
	if err != nil {
		return err
	}
	for *snapshot.State == ec2.SnapshotStatePending {
		log.Debugf("Snapshot %v process %v", *snapshot.SnapshotId, *snapshot.Progress)
		snapshot, err = s.GetSnapshot(snapshotID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ebsService) CreateSnapshot(request *CreateSnapshotRequest) (string, error) {
	params := &ec2.CreateSnapshotInput{
		VolumeId:    aws.String(request.VolumeID),
		Description: aws.String(request.Description),
	}
	resp, err := s.ec2Client.CreateSnapshot(params)
	if err != nil {
		return "", parseAwsError(err)
	}
	if request.Tags != nil {
		if err := s.AddTags(*resp.SnapshotId, request.Tags); err != nil {
			log.Warnf("Unable to tag %v with %v, but continue", *resp.SnapshotId, request.Tags)
		}
	}
	return *resp.SnapshotId, nil
}

func (s *ebsService) DeleteSnapshotWithRegion(snapshotID, region string) error {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
	}
	ec2Client := s.ec2Client
	if region != s.Region {
		ec2Client = ec2.New(session.New(), aws.NewConfig().WithRegion(region))
	}
	_, err := ec2Client.DeleteSnapshot(params)
	return parseAwsError(err)
}

func (s *ebsService) DeleteSnapshot(snapshotID string) error {
	return s.DeleteSnapshotWithRegion(snapshotID, s.Region)
}

func (s *ebsService) CopySnapshot(snapshotID, srcRegion string) (string, error) {
	// Copy to current region
	params := &ec2.CopySnapshotInput{
		SourceRegion:     aws.String(srcRegion),
		SourceSnapshotId: aws.String(snapshotID),
	}

	resp, err := s.ec2Client.CopySnapshot(params)
	if err != nil {
		return "", parseAwsError(err)
	}

	return *resp.SnapshotId, nil
}

func (s *ebsService) AddTags(resourceID string, tags map[string]string) error {
	if tags == nil {
		return nil
	}
	log.Debugf("Adding tags for %v, as %v", resourceID, tags)
	params := &ec2.CreateTagsInput{
		Resources: []*string{
			aws.String(resourceID),
		},
	}
	ec2Tags := []*ec2.Tag{}
	for k, v := range tags {
		tag := &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		ec2Tags = append(ec2Tags, tag)
	}
	params.Tags = ec2Tags

	_, err := s.ec2Client.CreateTags(params)
	if err != nil {
		return parseAwsError(err)
	}
	return nil
}

func (s *ebsService) DeleteTags(resourceID string, tags map[string]string) error {
	if tags == nil {
		return nil
	}
	log.Debugf("Deleting tags for %v, as %v", resourceID, tags)
	params := &ec2.DeleteTagsInput{
		Resources: []*string{
			aws.String(resourceID),
		},
	}
	ec2Tags := []*ec2.Tag{}
	for k, v := range tags {
		tag := &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		ec2Tags = append(ec2Tags, tag)
	}
	params.Tags = ec2Tags

	_, err := s.ec2Client.DeleteTags(params)
	if err != nil {
		return parseAwsError(err)
	}
	return nil
}

func (s *ebsService) GetTags(resourceID string) (map[string]string, error) {
	params := &ec2.DescribeTagsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("resource-id"),
				Values: []*string{
					aws.String(resourceID),
				},
			},
		},
	}

	resp, err := s.ec2Client.DescribeTags(params)
	if err != nil {
		return nil, parseAwsError(err)
	}

	result := map[string]string{}
	if resp.Tags == nil {
		return result, nil
	}

	for _, tag := range resp.Tags {
		if *tag.ResourceId != resourceID {
			return nil, util.NewConvoyDriverErr(errors.New("BUG: why the result is not related to what I asked for?"), util.ErrGenericFailureCode)
		}
		result[*tag.Key] = *tag.Value
	}
	return result, nil
}
