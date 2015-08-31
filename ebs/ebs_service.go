package ebs

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/service/ec2"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

const (
	GB = 1073741824
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "ebs"})
)

type ebsService struct {
	metadataClient *ec2metadata.Client
	ec2Client      *ec2.EC2
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
	s.metadataClient = ec2metadata.New(nil)
	if !s.IsEC2Instance() {
		return nil, fmt.Errorf("Not running on an EC2 instance")
	}

	region, err := s.GetRegion()
	if err != nil {
		return nil, err
	}
	config := aws.NewConfig().WithRegion(region)
	s.ec2Client = ec2.New(config)
	return s, nil
}

func (s *ebsService) IsEC2Instance() bool {
	return s.metadataClient.Available()
}

func (s *ebsService) GetRegion() (string, error) {
	return s.metadataClient.Region()
}

func (s *ebsService) GetAvailablityZone() (string, error) {
	return s.metadataClient.GetMetadata("placement/availability-zone")
}

func (s *ebsService) GetInstanceID() (string, error) {
	return s.metadataClient.GetMetadata("instance-id")
}

func (s *ebsService) waitForVolumeCreating(v *ec2.Volume) (*ec2.Volume, error) {
	volume := v
	if *volume.State == ec2.VolumeStateAvailable {
		return volume, nil
	}
	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			volume.VolumeId,
		},
	}
	for *volume.State == ec2.VolumeStateCreating {
		log.Debugf("Waiting for volume %v creating", *volume.VolumeId)
		time.Sleep(time.Second)
		volumes, err := s.ec2Client.DescribeVolumes(params)
		if err != nil {
			return nil, parseAwsError(err)
		}
		volume = volumes.Volumes[0]
	}
	return volume, nil
}

func (s *ebsService) CreateVolume(size int64, snapshotID, volumeType string) (string, error) {
	az, err := s.GetAvailablityZone()
	if err != nil {
		return "", err
	}
	// EBS size are in GB, we would round it up
	ebsSize := size / GB
	if size%GB > 0 {
		ebsSize += 1
	}

	params := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(az),
		Size:             aws.Int64(ebsSize),
	}
	if snapshotID != "" {
		params.SnapshotId = aws.String(snapshotID)
	}
	if volumeType != "" {
		if volumeType != "gp2" && volumeType != "io1" && volumeType != "standard" {
			return "", fmt.Errorf("Invalid volume type for EBS: %v", volumeType)
		}
		params.VolumeType = aws.String(volumeType)
	}

	ec2Volume, err := s.ec2Client.CreateVolume(params)
	if err != nil {
		return "", parseAwsError(err)
	}

	volumeID := *ec2Volume.VolumeId
	v, err := s.waitForVolumeCreating(ec2Volume)
	if *v.State != ec2.VolumeStateAvailable {
		log.Errorf("Failed creating volume, volume end state %v", *v.State)

		err = s.DeleteVolume(volumeID)
		if err != nil {
			log.Errorf("Failed deleting volume: %v", parseAwsError(err))
		}
		return "", fmt.Errorf("Failed creating volume with size %v and snapshot %v",
			size, snapshotID)
	}

	return *ec2Volume.VolumeId, nil
}

func (s *ebsService) DeleteVolume(volumeID string) error {
	params := &ec2.DeleteVolumeInput{
		VolumeId: aws.String(volumeID),
	}
	_, err := s.ec2Client.DeleteVolume(params)
	return parseAwsError(err)
}

func (s *ebsService) waitForVolumeAttaching(a *ec2.VolumeAttachment) (*ec2.VolumeAttachment, error) {
	attachment := a
	if *attachment.State == ec2.VolumeAttachmentStateAttached {
		return attachment, nil
	}
	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			attachment.VolumeId,
		},
	}
	for *attachment.State == ec2.VolumeAttachmentStateAttaching {
		log.Debugf("Waiting for volume %v attaching", *attachment.VolumeId)
		time.Sleep(time.Second)
		volumes, err := s.ec2Client.DescribeVolumes(params)
		if err != nil {
			return nil, parseAwsError(err)
		}
		volume := volumes.Volumes[0]
		if len(volume.Attachments) != 0 {
			attachment = volume.Attachments[0]
		} else {
			return nil, fmt.Errorf("Attaching failed for ", *attachment.VolumeId)
		}
	}
	return attachment, nil
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
	log.Debug(newDevList, oldDevList)
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
	return attachedDev, nil
}

func (s *ebsService) AttachVolume(volumeID, dev string, size int64) (string, error) {
	instanceID, err := s.GetInstanceID()
	if err != nil {
		return "", err
	}

	params := &ec2.AttachVolumeInput{
		Device:     aws.String(dev),
		InstanceId: aws.String(instanceID),
		VolumeId:   aws.String(volumeID),
	}

	blkList, err := getBlkDevList()
	if err != nil {
		return "", err
	}

	resp, err := s.ec2Client.AttachVolume(params)
	if err != nil {
		return "", parseAwsError(err)
	}

	resp, err = s.waitForVolumeAttaching(resp)
	if err != nil {
		return "", err
	}
	if *resp.State != ec2.VolumeAttachmentStateAttached {
		return "", fmt.Errorf("Cannot attach volume, final state %v", *resp.State)
	}

	result, err := getAttachedDev(blkList, size)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (s *ebsService) waitForVolumeDetaching(a *ec2.VolumeAttachment) error {
	attachment := a
	if *attachment.State == ec2.VolumeAttachmentStateDetached {
		return nil
	}
	params := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{
			attachment.VolumeId,
		},
	}
	for *attachment.State == ec2.VolumeAttachmentStateDetaching {
		log.Debugf("Waiting for volume %v detaching", *attachment.VolumeId)
		time.Sleep(time.Second)
		volumes, err := s.ec2Client.DescribeVolumes(params)
		if err != nil {
			return parseAwsError(err)
		}
		volume := volumes.Volumes[0]
		if len(volume.Attachments) != 0 {
			attachment = volume.Attachments[0]
		} else {
			// Already detached
			break
		}
	}
	return nil
}

func (s *ebsService) DetachVolume(volumeID string) error {
	instanceID, err := s.GetInstanceID()
	if err != nil {
		return err
	}

	params := &ec2.DetachVolumeInput{
		VolumeId:   aws.String(volumeID),
		InstanceId: aws.String(instanceID),
	}

	resp, err := s.ec2Client.DetachVolume(params)
	if err != nil {
		return parseAwsError(err)
	}

	return s.waitForVolumeDetaching(resp)
}

func (s *ebsService) waitForSnapshotComplete(snap *ec2.Snapshot) error {
	snapshot := snap
	if *snapshot.State == ec2.SnapshotStateCompleted {
		return nil
	}
	params := &ec2.DescribeSnapshotsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("snapshot-id"),
				Values: []*string{
					snapshot.SnapshotId,
				},
			},
		},
		OwnerIds: []*string{
			snapshot.OwnerId,
		},
		RestorableByUserIds: []*string{
			snapshot.OwnerId,
		},
		SnapshotIds: []*string{
			snapshot.SnapshotId,
		},
	}
	for *snapshot.State == ec2.SnapshotStatePending {
		log.Debugf("Snapshot %v process %v", *snapshot.SnapshotId, *snapshot.Progress)
		time.Sleep(time.Second)
		snapshots, err := s.ec2Client.DescribeSnapshots(params)
		if err != nil {
			return parseAwsError(err)
		}
		snapshot = snapshots.Snapshots[0]
	}
	return nil
}

func (s *ebsService) CreateSnapshot(volumeID, desc string) (string, error) {
	params := &ec2.CreateSnapshotInput{
		VolumeId:    aws.String(volumeID),
		Description: aws.String(desc),
	}
	resp, err := s.ec2Client.CreateSnapshot(params)
	if err != nil {
		return "", parseAwsError(err)
	}
	err = s.waitForSnapshotComplete(resp)
	if err != nil {
		return "", parseAwsError(err)
	}
	return *resp.SnapshotId, nil
}

func (s *ebsService) DeleteSnapshot(snapshotID string) error {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(snapshotID),
	}
	_, err := s.ec2Client.DeleteSnapshot(params)
	return parseAwsError(err)
}
