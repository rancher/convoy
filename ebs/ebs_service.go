package ebs

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
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
}

type CreateSnapshotRequest struct {
	VolumeID    string
	Description string
	Tags        map[string]string
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
		return fmt.Errorf(message)
	}
	return err
}

func NewEBSService() (*ebsService, error) {
	var err error

	s := &ebsService{}
	s.metadataClient = ec2metadata.New(session.New())
	if !s.isEC2Instance() {
		return nil, fmt.Errorf("Not running on an EC2 instance")
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
		log.Debugf("Got error to get volume %s", volumeID)
		return err
	}

	timeChan := time.NewTimer(time.Second * 61).C
	tickChan := time.NewTicker(time.Second * 3).C //tick at most 20 times before err out

	POLL:
	for *volume.State == start {
		select {
			case <- timeChan:
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
		return fmt.Errorf("Cannot finish volume %v state transition, ",
			"from %v to %v, though final state %v",
			volumeID, start, end, *volume.State)
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
			case <- timeChan:
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
					return fmt.Errorf("Attaching failed for %s", volumeID)
				}
		}
	}
	if *attachment.State != ec2.VolumeAttachmentStateAttached {
		return fmt.Errorf("Cannot attach volume, final state %v", *attachment.State)
	}
	return nil
}

func (s *ebsService) CreateVolume(request *CreateEBSVolumeRequest) (string, error) {
	if request == nil {
		return "", fmt.Errorf("Invalid CreateEBSVolumeRequest")
	}
	size := request.Size
	iops := request.IOPS
	snapshotID := request.SnapshotID
	volumeType := request.VolumeType

	// EBS size are in GB, we would round it up
	ebsSize := size / GB
	if size%GB > 0 {
		ebsSize += 1
	}

	params := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(s.AvailabilityZone),
		Size:             aws.Int64(ebsSize),
	}
	if snapshotID != "" {
		params.SnapshotId = aws.String(snapshotID)
	}
	if volumeType != "" {
		if volumeType != "gp2" && volumeType != "io1" && volumeType != "standard" {
			return "", fmt.Errorf("Invalid volume type for EBS: %v", volumeType)
		}
		if volumeType == "io1" && iops == 0 {
			return "", fmt.Errorf("Invalid IOPS for volume type io1")
		}
		if volumeType != "io1" && iops != 0 {
			return "", fmt.Errorf("IOPS only valid for volume type io1")
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
		return "", fmt.Errorf("Failed creating volume with size %v and snapshot %v",
			size, snapshotID)
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
		return nil, fmt.Errorf("Cannot find any volumes")
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
		return nil, fmt.Errorf("Cannot find volume %v", volumeID)
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
	// Since tag Name is not AWS's identifying attribute (i.e. volume_id), we can get multiple results with same name
	// Return the first one
	if len(volumes.Volumes) < 1 {
		return nil, fmt.Errorf("Cannot find volume by name %s in region %s in az %s", volumeName, s.Region, s.AvailabilityZone)
	}else if len(volumes.Volumes) > 1 {
		log.Debugf("Found multiple volumes with name %s. Returning the first one.", volumeName)
	}
	return volumes.Volumes[0], nil
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
		return "", fmt.Errorf("Cannot find a device matching description")
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
	return "", fmt.Errorf("Cannot find an available device for instance %v", s.InstanceID)
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
			Force:aws.Bool(true),
		}

		if _, err := s.ec2Client.DetachVolume(forceDetachParams); err != nil {
			return "", parseAwsError(err)
		}

		forceDetachErr := s.waitForVolumeTransition(volumeID, ec2.VolumeAttachmentStateAttaching, ec2.VolumeStateAvailable)

		if forceDetachErr != nil{
			log.Errorf("Error in force detach's state transition: %s - Returning the error", forceDetachErr.Error())
			return "", fmt.Errorf("Force Detach Err: %s", forceDetachErr.Error())
		}

		fdTag := make(map[string]string)
		fdTag["ForceDetached"] = "true"
		s.AddTags(s.InstanceID, fdTag)

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
			Force:aws.Bool(true),
		}

		if _, err := s.ec2Client.DetachVolume(forceDetachParams); err != nil {
			return parseAwsError(err)
		}

		forceDetachErr := s.waitForVolumeTransition(volumeID, ec2.VolumeStateInUse, ec2.VolumeStateAvailable)

		if forceDetachErr != nil{
			log.Errorf("Error in force detach's state transition: %s - Returning the error", forceDetachErr.Error())
			return fmt.Errorf("Force Detach Err: %s", forceDetachErr.Error())
		}

		fdTag := make(map[string]string)
		fdTag["ForceDetached"] = "true"
		s.AddTags(s.InstanceID, fdTag)

		log.Debugf("Successfully force detached the volume %s", volumeID)
		return forceDetachErr
	}
	log.Debugf("Successfully detached the volume %s", volumeID)
	return detachErr
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
		return nil, fmt.Errorf("Cannot find snapshot %v", snapshotID)
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
			return nil, fmt.Errorf("BUG: why the result is not related to what I asked for?")
		}
		result[*tag.Key] = *tag.Value
	}
	return result, nil
}
