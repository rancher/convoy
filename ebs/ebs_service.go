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
	RETRY_INTERVAL = 1
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

	config := aws.NewConfig().WithRegion(s.Region).WithMaxRetries(5)
	s.ec2Client = ec2.New(session.New(), config)

	return s, nil
}

func (s *ebsService) isEC2Instance() bool {
	return s.metadataClient.Available()
}

func (s *ebsService) waitForVolumeTransition(volumeID, start, end string) error {
	volume, err := s.GetVolume(volumeID)
	if err != nil {
		return err
	}

	for *volume.State == start {
		log.Debugf("Waiting for volume %v state transiting from %v to %v",
			volumeID, start, end)
		sleepBeforeRetry()
		volume, err = s.GetVolume(volumeID)
		if err != nil {
			return err
		}
	}
	if *volume.State != end {
		return fmt.Errorf("Cannot finish volume %v state transition, from %v to %v, though final state %v",
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
		sleepBeforeRetry()
		volume, err = s.GetVolume(volumeID)
		if err != nil {
			return err
		}
	}
	attachment = volume.Attachments[0]

	for *attachment.State == ec2.VolumeAttachmentStateAttaching {
		log.Debugf("Waiting for volume %v attaching", volumeID)
		sleepBeforeRetry()
		volume, err := s.GetVolume(volumeID)
		if err != nil {
			return err
		}
		if len(volume.Attachments) != 0 {
			attachment = volume.Attachments[0]
		} else {
			return fmt.Errorf("Attaching failed for ", volumeID)
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
			return "", err
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

func (s *ebsService) GetVolume(volumeID string) (*ec2.Volume, error) {
	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			aws.String(volumeID),
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

	return s.waitForVolumeTransition(volumeID, ec2.VolumeStateInUse, ec2.VolumeStateAvailable)
}

func (s *ebsService) GetSnapshotWithRegion(snapshotID, region string) (*ec2.Snapshot, error) {
	params := &ec2.DescribeSnapshotsInput{
		SnapshotIds: []*string{
			aws.String(snapshotID),
		},
	}
	ec2Client := s.ec2Client
	if region != s.Region {
		ec2Client = ec2.New(session.New(), aws.NewConfig().WithRegion(region).WithMaxRetries(5))
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
	return s.GetSnapshotWithRegion(snapshotID, s.Region)
}

func (s *ebsService) WaitForSnapshotComplete(snapshotID string) error {
	snapshot, err := s.GetSnapshot(snapshotID)
	if err != nil {
		return err
	}
	for *snapshot.State == ec2.SnapshotStatePending {
		log.Debugf("Snapshot %v process %v", *snapshot.SnapshotId, *snapshot.Progress)
		sleepBeforeRetry()
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
		ec2Client = ec2.New(session.New(), aws.NewConfig().WithRegion(region).WithMaxRetries(5))
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
