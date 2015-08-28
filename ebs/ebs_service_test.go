//+build ebstest

package ebs

import (
	"github.com/Sirupsen/logrus"
	"os"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)
}

func (s *TestSuite) TestEC2Metadata(c *C) {
	var err error

	svc, err := NewEBSService()
	c.Assert(err, IsNil)

	isEC2 := svc.IsEC2Instance()
	c.Assert(isEC2, Equals, true)

	region, err := svc.GetRegion()
	c.Assert(err, IsNil)
	c.Assert(region, Not(Equals), "")

	az, err := svc.GetAvailablityZone()
	c.Assert(err, IsNil)
	c.Assert(az, Not(Equals), "")

	instanceID, err := svc.GetInstanceID()
	c.Assert(err, IsNil)
	c.Assert(instanceID, Not(Equals), "")
}

func (s *TestSuite) TestVolumeAndSnapshot(c *C) {
	var err error

	svc, err := NewEBSService()
	c.Assert(err, IsNil)

	logrus.Debug("Creating volume1")
	volumeID1, err := svc.CreateVolume(GB, "", "")
	c.Assert(err, IsNil)
	c.Assert(volumeID1, Not(Equals), "")

	logrus.Debug("Attaching volume1")
	dev1, err := svc.AttachVolume(volumeID1, "/dev/sdf")
	c.Assert(err, IsNil)
	c.Assert(dev1, Not(Equals), "")
	logrus.Debug("Attached volume1 at ", dev1)

	logrus.Debug("Creating snapshot")
	snapshotID, err := svc.CreateSnapshot(volumeID1, "Test snapshot")
	c.Assert(err, IsNil)
	c.Assert(snapshotID, Not(Equals), "")
	logrus.Debug("Created snapshot ", snapshotID)

	logrus.Debug("Creating volume from snapshot")
	volumeID2, err := svc.CreateVolume(2*GB, snapshotID, "gp2")
	c.Assert(err, IsNil)
	c.Assert(volumeID2, Not(Equals), "")

	logrus.Debug("Deleting snapshot")
	err = svc.DeleteSnapshot(snapshotID)
	c.Assert(err, IsNil)

	logrus.Debug("Attaching volume2")
	dev2, err := svc.AttachVolume(volumeID2, "/dev/sdg")
	c.Assert(err, IsNil)
	c.Assert(dev2, Not(Equals), "")
	logrus.Debug("Attached volume2 at ", dev2)

	logrus.Debug("Detaching volume2")
	err = svc.DetachVolume(volumeID2)
	c.Assert(err, IsNil)

	logrus.Debug("Detaching volume1")
	err = svc.DetachVolume(volumeID1)
	c.Assert(err, IsNil)

	logrus.Debug("Deleting volume2")
	err = svc.DeleteVolume(volumeID2)
	c.Assert(err, IsNil)

	logrus.Debug("Deleting volume1")
	err = svc.DeleteVolume(volumeID1)
	c.Assert(err, IsNil)
}
