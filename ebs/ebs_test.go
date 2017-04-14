package ebs

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/rancher/convoy/convoydriver"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
	"time"
	"math/rand"
)

var driver *Driver

const (
	MOCK_VOLUME_SIZE_IN_GB = int64(5)
	MOCK_VOLUME_SIZE = MOCK_VOLUME_SIZE_IN_GB * GB
	MOCK_VOLUME_ID = "volumeId"
	MOCK_VOLUME_NAME = "volume-name"
)

var MOCK_BUILD_RETURN = BuildReturn {
		volumeSize: MOCK_VOLUME_SIZE,
		volumeId: MOCK_VOLUME_ID,
	}

func TestMain(m *testing.M) {
	driver = &Driver{
		mutex: new(sync.RWMutex),
	}
	code := m.Run()
	os.Exit(code)
}

func getNewTags() map[string]string {
	newTags := map[string]string{
		"Name":   MOCK_VOLUME_NAME,
		"DCName": "DC",
	}
	return newTags
}

func getVolume(volumeId string) *ec2.Volume {
	size :=  MOCK_VOLUME_SIZE_IN_GB
	az := "az-1"
	var t time.Time

	volume := &ec2.Volume{
		Size:             &size,
		VolumeId:         &volumeId,
		AvailabilityZone: &az,
		CreateTime:       &t,
		Encrypted:        aws.Bool(true),
	}
	return volume
}

func getSnapshot(snapshotId string) *ec2.Snapshot {
	var t time.Time
	snapshot := &ec2.Snapshot{
		SnapshotId: &snapshotId,
		StartTime:  &t,
		Encrypted:  aws.Bool(true),
	}
	return snapshot
}

func TestFailureIfNoAZs(t *testing.T) {
	newTags := getNewTags()

	// Setup the mock functions
	ebsMock := NewEbsMock()
	driver.ebsService = ebsMock

	// Should fail because no availability zones are defined
	buildReturn, err := driver.BuildVolume(MOCK_VOLUME_NAME, MOCK_VOLUME_ID, map[string]string{}, newTags)
	require.NotNil(t, err)
	require.Nil(t, buildReturn)
}

func TestMostRecentVolume(t *testing.T) {
	newTags := getNewTags()

	// Setup the mock functions
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}
	driver.ebsService = ebsMock

	// Should fail because a volumeID is defined, but there is no Volume with its id
	buildReturn, err := driver.BuildVolume(MOCK_VOLUME_NAME, MOCK_VOLUME_ID, map[string]string{}, newTags)
	require.NotNil(t, err)
	require.Nil(t, buildReturn)
}

func TestFailoverOptOut(t *testing.T) {
	newTags := getNewTags()

	// Setup the mock functions
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}

	volume := getVolume(MOCK_VOLUME_ID)
	volume.Tags = []*ec2.Tag{&ec2.Tag{Key: aws.String("Failover"), Value: aws.String("False")}}
	ebsMock.VolumeMapById[MOCK_VOLUME_ID] = volume
	ebsMock.MostRecentVolume = volume
	driver.ebsService = ebsMock

	// Should exit nicely with original volume values
	buildReturn, err := driver.BuildVolume(MOCK_VOLUME_NAME, MOCK_VOLUME_ID, map[string]string{}, newTags)
	require.Nil(t, err)
	require.Equal(t, MOCK_BUILD_RETURN, *buildReturn)

	// Verify lowercase
	volume.Tags = []*ec2.Tag{&ec2.Tag{Key: aws.String("Failover"), Value: aws.String("false")}}

	// Should fail because a volumeID is defined, but there is no Volume with its id
	buildReturn, err = driver.BuildVolume(MOCK_VOLUME_NAME, MOCK_VOLUME_ID, map[string]string{}, newTags)
	require.Nil(t, err)
	require.Equal(t, MOCK_BUILD_RETURN, *buildReturn)
}

func TestBuildVolumeFromScratch(t *testing.T) {
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}
	ebsMock.AvailabilityZone = "az-1"
	driver.ebsService = ebsMock

	opts := make(map[string]string)
	opts[OPT_VOLUME_NAME] = MOCK_VOLUME_NAME
	opts[OPT_SIZE] = "5G"
	opts[OPT_VOLUME_TYPE] = "gp2"

	args := &BuildArgs{
		volumeName: MOCK_VOLUME_NAME,
		volumeId:   MOCK_VOLUME_ID,
		opts:       opts,
		tags:       map[string]string{},
	}
	// Should return new volume if no snapshot reference and no volume reference
	buildReturn, err := driver.BuildVolumeFromScratch(nil, nil, args)
	require.Nil(t, err)
	require.Equal(t, "newVolumeId", buildReturn.volumeId)
	require.Equal(t, MOCK_VOLUME_SIZE, buildReturn.volumeSize)

	// If there is a volume that should failover in the SAME AZ then return no buildReturn and no error
	volume := getVolume(MOCK_VOLUME_ID)
	buildReturn, err = driver.BuildVolumeFromScratch(volume, nil, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	// If there is a volume that should failover in ANOTHER AZ then return no buildReturn and no error
	volume.AvailabilityZone = aws.String("az-2")
	buildReturn, err = driver.BuildVolumeFromScratch(volume, nil, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	// If there is a volume that SHOULD NOT failover in the SAME AZ then return no buildReturn and no error
	volume.AvailabilityZone = aws.String("az-1")
	volume.Tags = []*ec2.Tag{&ec2.Tag{Key: aws.String("Failover"), Value: aws.String("false")}}
	buildReturn, err = driver.BuildVolumeFromScratch(volume, nil, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	// If there is a volume that SHOULD NOT failover in ANOTHER AZ then return a new volume
	volume.AvailabilityZone = aws.String("az-2")
	volume.Tags = []*ec2.Tag{&ec2.Tag{Key: aws.String("Failover"), Value: aws.String("false")}}
	buildReturn, err = driver.BuildVolumeFromScratch(volume, nil, args)
	require.Nil(t, err)
	require.Equal(t, "newVolumeId", buildReturn.volumeId)
	require.Equal(t, int64(MOCK_VOLUME_SIZE), buildReturn.volumeSize)
}

func TestMountLocalVolume(t *testing.T) {
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}
	ebsMock.AvailabilityZone = "az-1"
	driver.ebsService = ebsMock

	opts := make(map[string]string)
	opts[OPT_VOLUME_NAME] = "volume-name"
	opts[OPT_SIZE] = "5G"
	opts[OPT_VOLUME_TYPE] = "gp2"

	args := &BuildArgs{
		volumeName: MOCK_VOLUME_NAME,
		volumeId:   MOCK_VOLUME_ID,
		opts:       opts,
		tags:       map[string]string{},
	}

	// Should return new volume if no snapshot reference and no volume reference
	buildReturn, err := driver.MountLocalVolume(nil, nil, args)
	require.Nil(t, buildReturn)

	snapshot := getSnapshot("snapshotId")
	snapshot.VolumeId = aws.String("diff-volume")
	volume := getVolume(MOCK_VOLUME_ID)
	volume.AvailabilityZone = aws.String("az-2")

	// If volume is in other AZ, then return an no buildReturn and no error
	buildReturn, err = driver.MountLocalVolume(volume, snapshot, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	// If volume is in same AZ, but snapshot is nil then return no buildReturn and no error
	buildReturn, err = driver.MountLocalVolume(volume, snapshot, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	// If volume is in same AZ, but snapshot is nil then return volume
	volume.AvailabilityZone = aws.String("az-1")
	buildReturn, err = driver.MountLocalVolume(volume, nil, args)
	require.Nil(t, err)
	require.Equal(t, MOCK_BUILD_RETURN, *buildReturn)

	// If volume is in same AZ and snapshot is of same volume then return volume
	snapshot.VolumeId = aws.String(MOCK_VOLUME_ID)
	buildReturn, err = driver.MountLocalVolume(volume, snapshot, args)
	require.Nil(t, err)
	require.Equal(t, MOCK_BUILD_RETURN, *buildReturn)

	// If volume is in same AZ and snapshot is of different volume, but volume is more up to date
	snapshot.VolumeId = aws.String("diff-volume")
	volume.CreateTime = aws.Time(time.Now())
	buildReturn, err = driver.MountLocalVolume(volume, snapshot, args)
	require.Nil(t, err)
	require.Equal(t, MOCK_BUILD_RETURN, *buildReturn)
}

func TestRebuildFromRemoteVolume(t *testing.T) {
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}
	ebsMock.AvailabilityZone = "az-1"
	driver.ebsService = ebsMock

	opts := make(map[string]string)
	opts[OPT_VOLUME_NAME] = "volume-name"
	opts[OPT_SIZE] = "5G"
	opts[OPT_VOLUME_TYPE] = "gp2"

	args := &BuildArgs{
		volumeName: MOCK_VOLUME_NAME,
		volumeId:   MOCK_VOLUME_ID,
		opts:       opts,
		tags:       map[string]string{},
	}

	// If the volume is nil then return no buildReturn and no error
	buildReturn, err := driver.RebuildFromRemoteVolume(nil, nil, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	volumeId := "volume-id"
	volume := getVolume(volumeId)
	volume.AvailabilityZone = aws.String("az-2")
	ebsMock.VolumeMapById["volume-id"] = volume

	// Volume is in other AZ and should failover
	buildReturn, err = driver.RebuildFromRemoteVolume(volume, nil, args)
	require.Nil(t, err)
	require.Equal(t, int64(MOCK_VOLUME_SIZE), buildReturn.volumeSize)
}

func TestRebuildFromSnapshot(t *testing.T) {
	ebsMock := NewEbsMock()
	ebsMock.AvailabilityZones = []*string{aws.String("az-1"), aws.String("az-2")}
	ebsMock.AvailabilityZone = "az-1"
	driver.ebsService = ebsMock

	opts := make(map[string]string)
	opts[OPT_VOLUME_NAME] = MOCK_VOLUME_NAME
	opts[OPT_SIZE] = "5G"
	opts[OPT_VOLUME_TYPE] = "gp2"

	args := &BuildArgs{
		volumeName: MOCK_VOLUME_NAME,
		volumeId:   MOCK_VOLUME_ID,
		opts:       opts,
		tags:       map[string]string{},
	}

	// If no snapshot is specified then return no buildReturn and no error
	buildReturn, err := driver.RebuildFromSnapshot(nil, nil, args)
	require.Nil(t, err)
	require.Nil(t, buildReturn)

	snapshot := getSnapshot("snapshotId")
	snapshot.VolumeId = aws.String("diff-volume")
	snapshot.VolumeSize = aws.Int64(MOCK_VOLUME_SIZE_IN_GB) // 5 GB volume

	// Volume is nil so should rebuild from snapshot
	buildReturn, err = driver.RebuildFromSnapshot(nil, snapshot, args)
	require.Nil(t, err)
	require.Equal(t, int64(MOCK_VOLUME_SIZE), buildReturn.volumeSize)

	// Volume must be in current AZ, NOT the volume the snapshot is from, and the snapshot is created before the volume
	// This ensures that the snapshot is of a volume that no longer exists due to AZ outage (or manual deletion)
	volume := getVolume(MOCK_VOLUME_ID)
	snapshot.StartTime = aws.Time(time.Now())
	buildReturn, err = driver.RebuildFromSnapshot(volume, snapshot, args)
	require.Nil(t, err)
	require.Equal(t, int64(MOCK_VOLUME_SIZE), buildReturn.volumeSize)
}

func TestRandomCombinations(t *testing.T) {
	rand.Seed(time.Now().Unix())
	newTags := getNewTags()
	// Setup the mock functions
	ebsMock := NewEbsMock()
	azs := []*string{aws.String("az-1"), aws.String("az-2")}
	ebsMock.AvailabilityZones = azs
	driver.ebsService = ebsMock

	opts := make(map[string]string)
	opts[OPT_VOLUME_NAME] = MOCK_VOLUME_NAME
	opts[OPT_SIZE] = "5G"
	opts[OPT_VOLUME_TYPE] = "gp2"

	// All four cases of snapshot, volume combos
	// (nil, nil) (volume,nil) (nil, snapshot), (volume, snapshot)
	for i := 0; i < 4; i++ {
		var volume *ec2.Volume
		var snapshot *ec2.Snapshot
		if i == 1 || i == 3 {
			volume = getVolume(MOCK_VOLUME_ID)
			volume.AvailabilityZone = azs[rand.Intn(len(azs))]
			ebsMock.SetMostRecentVolume(volume)
		}
		if i == 2 || i == 3 {
			snapshot = getSnapshot("snapshot-id")
			volumeId := MOCK_VOLUME_ID
			volumeSize := MOCK_VOLUME_SIZE_IN_GB
			snapshot.VolumeId = &volumeId
			snapshot.VolumeSize = &volumeSize
			ebsMock.SetMostRecentSnapshot(snapshot)
		}

		for _, az := range azs {
			if volume != nil {
				volume.AvailabilityZone = az
			}
			// The cases of the volume.CreatedTime vs snapshot.StartTime
			// lt, eq, gt 
			for j := 0; j < 3; j++ {
				if volume != nil && snapshot != nil {
					now := time.Now()
					snapshot.StartTime = aws.Time(now)
					switch(j) {
					case 0:
						volume.CreateTime = aws.Time(now.Add(-5 * time.Minute))
					case 1:
						volume.CreateTime = aws.Time(now)
					case 2:
						volume.CreateTime = aws.Time(now.Add(5 * time.Minute))
					}
				}

				if volume == nil {
					buildReturn, err := driver.BuildVolume(MOCK_VOLUME_NAME, "", opts, newTags)
					require.Nil(t, err)
					require.NotNil(t, buildReturn)
				} else {
					buildReturn, err := driver.BuildVolume(MOCK_VOLUME_NAME, MOCK_VOLUME_ID, opts, newTags)
					require.Nil(t, err)
					require.NotNil(t, buildReturn)
				}

			} 
		}
	}
}

