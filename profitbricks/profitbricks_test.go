package profitbricks

import (
	"os"
	"testing"

	"github.com/Sirupsen/logrus"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct{}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)
}

func (s *TestSuite) TestProfitBricksMetadata(c *C) {
	_, err := InitClient()
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestVolumeAndSnapshot(c *C) {
	svc, err := InitClient()
	c.Assert(err, IsNil)

  // Simple server creation here.  More complex test cases in TestComplexVolume
  logrus.Debug("Creating blank volume")
  var (
    volumeName = "test_volume"
    volumeSize = int(1)
    volumeType = "HDD"
  )
  volumeParams := CreateVolumeParams{
    Name: volumeName,
    Size: volumeSize,
    Type: volumeType,
  }
  volume, err := svc.CreateVolume(volumeParams)
	c.Assert(err, IsNil)
	c.Assert(volume.Name, Equals, volumeName)
  c.Assert(volume.Size, Equals, volumeSize)
  c.Assert(volume.Type, Equals, volumeType)

	logrus.Debug("Attaching volume")
	volume, err = svc.AttachVolume(volume.Id)
	c.Assert(err, IsNil)
  c.Assert(volume.DeviceNumber, Not(Equals), IsNil)

  logrus.Debug("Checking device path")
  deviceSuffix := svc.GetDeviceSuffix(volume.DeviceNumber)
  devicePath := BASE_DEVICE_PATH + deviceSuffix
	stat1, err := os.Stat(devicePath)
	c.Assert(err, IsNil)
	c.Assert(stat1.Mode()&os.ModeDevice != 0, Equals, true)

  logrus.Debug("Testing GET volume")
  volume, err = svc.GetVolume(volume.Id)
  c.Assert(err, IsNil)
  c.Assert(volume.Name, Equals, volumeName)
  c.Assert(volume.Size, Equals, volumeSize)
  c.Assert(volume.Type, Equals, volumeType)

  logrus.Debug("Creating snapshot")
  snapshotName := "test_snapshot"
  snapshot, err := svc.CreateSnapshot(volume.Id, snapshotName)
  c.Assert(err, IsNil)
  c.Assert(snapshot.Name, Equals, snapshotName)
  c.Assert(snapshot.Size, Equals, volume.Size)

  logrus.Debug("Testing GET snapshot")
  snapshot, err = svc.GetSnapshot(snapshot.Id)
  c.Assert(err, IsNil)
  c.Assert(snapshot.Name, Equals, snapshotName)
  c.Assert(snapshot.Size, Equals, volume.Size)

  logrus.Debug("Deleting snapshot")
	err = svc.DeleteSnapshot(snapshot.Id)
	c.Assert(err, IsNil)

	logrus.Debug("Deleting volume")
	err = svc.DeleteVolume(volume.Id)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestComplexVolume(c *C) {
	svc, err := InitClient()
	c.Assert(err, IsNil)

  logrus.Debug("Creating blank volume")
  var (
    volumeName = "blank_volume"
    volumeSize = int(1)
    volumeType = "HDD"
  )
  volumeParams := CreateVolumeParams{
    Name: volumeName,
    Size: volumeSize,
    Type: volumeType,
  }
  blankVolume, err := svc.CreateVolume(volumeParams)
	c.Assert(err, IsNil)
	c.Assert(blankVolume.Name, Equals, volumeName)
  c.Assert(blankVolume.Size, Equals, volumeSize)
  c.Assert(blankVolume.Type, Equals, volumeType)

  logrus.Debug("Creating snapshot")
  snapshotName := "test_snapshot"
  snapshot, err := svc.CreateSnapshot(blankVolume.Id, snapshotName)
  c.Assert(err, IsNil)
  c.Assert(snapshot.Name, Equals, snapshotName)
  c.Assert(snapshot.Size, Equals, blankVolume.Size)

  logrus.Debug("Creating volume from snapshot")
  volumeName = "from_snapshot"
  volumeParams = CreateVolumeParams{
    Name: volumeName,
    Size: volumeSize,
    Type: volumeType,
    SnapshotId: snapshot.Id,
  }
  fromSnapshot, err := svc.CreateVolume(volumeParams)
	c.Assert(err, IsNil)
	c.Assert(fromSnapshot.Name, Equals, volumeName)
  c.Assert(fromSnapshot.Size, Equals, volumeSize)
  c.Assert(fromSnapshot.Type, Equals, volumeType)

  logrus.Debug("Adding pre-existing volume to Convoy")
  volumeName = "existing_volume"
  volumeParams = CreateVolumeParams{
    Name: volumeName,
    Id: blankVolume.Id,
  }
  existingVolume, err := svc.CreateVolume(volumeParams)
	c.Assert(err, IsNil)
	c.Assert(existingVolume.Name, Equals, volumeName)
  c.Assert(existingVolume.Size, Equals, volumeSize)
  c.Assert(existingVolume.Type, Equals, volumeType)

  logrus.Debug("Deleting snapshot")
	err = svc.DeleteSnapshot(snapshot.Id)
	c.Assert(err, IsNil)

  logrus.Debug("Deleting complex volumes")
	err = svc.DeleteVolume(fromSnapshot.Id)
	c.Assert(err, IsNil)
  err = svc.DeleteVolume(existingVolume.Id)
	c.Assert(err, IsNil)
}
