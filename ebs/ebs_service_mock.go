package ebs

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type EbsMock struct {
	InstanceId       string
	AvailabilityZone string
	Region           string

	AvailabilityZones []*string

	VolumeMapById   map[string]*ec2.Volume
	VolumeMapByName map[string]*ec2.Volume

	SnapshotMapById map[string]*ec2.Snapshot

	SnapshotsMapByName map[string][]*ec2.Snapshot

	MostRecentSnapshot *ec2.Snapshot
	MostRecentVolume   *ec2.Volume

	Snapshot *ec2.Snapshot

	*Device
}

func NewEbsMock() *EbsMock {
	device, err := getDefaultDevice("root", map[string]string{})
	if err != nil {
		panic(err)
	}
	return &EbsMock{
		Device:             device,
		VolumeMapById:      make(map[string]*ec2.Volume),
		VolumeMapByName:    make(map[string]*ec2.Volume),
		SnapshotMapById:    make(map[string]*ec2.Snapshot),
		SnapshotsMapByName: make(map[string][]*ec2.Snapshot),
	}
}

func (e *EbsMock) getVolumeById(id string) (*ec2.Volume, error) {
	vol, ok := e.VolumeMapById[id]
	if !ok {
		return nil, fmt.Errorf("Volume %s does not exist", id)
	}
	return vol, nil
}

func (e *EbsMock) getVolumeByName(name string) (*ec2.Volume, error) {
	vol, ok := e.VolumeMapByName[name]
	if !ok {
		return nil, fmt.Errorf("Volume %s does not exist", name)
	}
	return vol, nil
}

func (e *EbsMock) getSnapshotById(id string) (*ec2.Snapshot, error) {
	snap, ok := e.SnapshotMapById[id]
	if !ok {
		return nil, fmt.Errorf("Snapshot %s does not exist", id)
	}
	return snap, nil
}

func (e *EbsMock) getSnapshotsByName(name string) ([]*ec2.Snapshot, error) {
	snaps, ok := e.SnapshotsMapByName[name]
	if !ok {
		return snaps, fmt.Errorf("Snapshot %s does not exist", name)
	}
	return snaps, nil
}

func (e *EbsMock) GetInstanceID() string {
	return e.InstanceId
}

func (e *EbsMock) GetRegion() string {
	return e.Region
}

func (e *EbsMock) GetAvailabilityZone() string {
	return e.AvailabilityZone
}

func (e *EbsMock) GetAvailabilityZones(...*ec2.Filter) ([]*string, error) {
	return e.AvailabilityZones, nil
}

func (e *EbsMock) CreateVolume(createVolume *CreateEBSVolumeRequest) (string, error) {
	return "newVolumeId", nil
}

func (e *EbsMock) DeleteVolume(id string) error {
	_, err := e.getVolumeById(id)
	if err != nil {
		return err
	}
	delete(e.VolumeMapById, id)
	return nil
}

func (e *EbsMock) GetVolumes(ids []string) ([]*ec2.Volume, error) {
	var volumes []*ec2.Volume
	for _, id := range ids {
		vol, err := e.getVolumeById(id)
		if err != nil {
			return volumes, err
		}
		volumes = append(volumes, vol)
	}
	return volumes, nil
}

func (e *EbsMock) GetVolume(id string) (*ec2.Volume, error) {
	return e.getVolumeById(id)
}

func (e *EbsMock) GetVolumeByName(name string, dcName string) (*ec2.Volume, error) {
	return e.getVolumeByName(name)
}

func (e *EbsMock) FindFreeDeviceForAttach() (string, error) {
	return "/dev/sda", nil
}

func (e *EbsMock) AttachVolume(id string, size int64) (string, error) {
	return "/dev/sda", nil
}

func (e *EbsMock) DetachVolume(id string) error {
	return nil
}

func (e *EbsMock) SetMostRecentSnapshot(snapshot *ec2.Snapshot) {
	e.SnapshotMapById[*snapshot.SnapshotId] = snapshot
	e.MostRecentSnapshot = snapshot
}

func (e *EbsMock) GetMostRecentSnapshot(string, string, ...*ec2.Filter) (*ec2.Snapshot, error) {
	return e.MostRecentSnapshot, nil
}

func (e *EbsMock) SetMostRecentVolume(volume *ec2.Volume) {
	e.VolumeMapById[*volume.VolumeId] = volume
	e.MostRecentVolume = volume
}
func (e *EbsMock) GetMostRecentAvailableVolume(string, string, ...*ec2.Filter) (*ec2.Volume, error) {
	return e.MostRecentVolume, nil
}

func (e *EbsMock) LaunchSnapshot(volumeId string, desc string, tag map[string]string) (string, error) {
	volume, ok := e.VolumeMapById[volumeId]
	if !ok {
		return "", fmt.Errorf("Volume %s does not exist", volumeId)
	}
	snapshot := &ec2.Snapshot{
		VolumeSize: volume.Size,
		VolumeId:   aws.String(volumeId),
		SnapshotId: aws.String("snap-a"),
		Encrypted:  volume.Encrypted,
	}
	e.SnapshotMapById["snap-a"] = snapshot
	return "snap-a", nil
}

func (e *EbsMock) GetSnapshots(name string, dc string, filters ...*ec2.Filter) ([]*ec2.Snapshot, error) {
	snapshots, err := e.getSnapshotsByName(name)
	return snapshots, err
}

func (e *EbsMock) GetSnapshotWithRegion(string, string) (*ec2.Snapshot, error) {
	return e.Snapshot, nil
}

func (e *EbsMock) GetSnapshot(id string) (*ec2.Snapshot, error) {
	return e.getSnapshotById(id)
}

func (e *EbsMock) WaitForSnapshotComplete(snapshotID string) error {
	return nil
}

func (e *EbsMock) CreateSnapshot(createSnap *CreateSnapshotRequest) (string, error) {
	return "", nil
}

func (e *EbsMock) DeleteSnapshotWithRegion(id string, region string) error {
	return e.DeleteSnapshot(id)
}

func (e *EbsMock) DeleteSnapshot(id string) error {
	_, err := e.getSnapshotById(id)
	if err != nil {
		return err
	}
	delete(e.SnapshotMapById, id)
	return nil
}

func (e *EbsMock) CopySnapshot(string, string) (string, error) {
	return "snap-b", nil
}

func (e *EbsMock) AddTags(string, map[string]string) error {
	return nil
}

func (e *EbsMock) DeleteTags(string, map[string]string) error {
	return nil
}

func (e *EbsMock) GetTags(string) (map[string]string, error) {
	return map[string]string{}, nil
}
