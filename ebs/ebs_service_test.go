//+build ebstest

package ebs

import (
	"os"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"

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

	c.Assert(svc.Region, Not(Equals), "")
	c.Assert(svc.AvailabilityZone, Not(Equals), "")
	c.Assert(svc.InstanceID, Not(Equals), "")
}

func (s *TestSuite) TestBlkDevList(c *C) {
	devList, err := getBlkDevList()
	c.Assert(err, IsNil)
	c.Assert(len(devList), Not(Equals), 0)
	c.Assert(devList["loop0"], Equals, true)
}

func (s *TestSuite) TestVolumeAndSnapshot(c *C) {
	var (
		err  error
		tags map[string]string
	)

	svc, err := NewEBSService()
	c.Assert(err, IsNil)

	// should contain the root device only
	devMap, err := svc.getInstanceDevList()
	c.Assert(err, IsNil)
	originDevCounts := len(devMap)
	c.Assert(originDevCounts, Not(Equals), 0)

	log.Debug("Creating volume1")
	r1Tags := map[string]string{
		"Name": "volume1",
	}
	r1 := &CreateEBSVolumeRequest{
		Size: GB,
		Tags: r1Tags,
	}
	volumeID1, err := svc.CreateVolume(r1)
	c.Assert(err, IsNil)
	c.Assert(volumeID1, Not(Equals), "")
	tags, err = svc.GetTags(volumeID1)
	c.Assert(err, IsNil)
	c.Assert(r1Tags, DeepEquals, tags)

	log.Debug("Attaching volume1")
	dev1, err := svc.AttachVolume(volumeID1, GB)
	c.Assert(err, IsNil)
	c.Assert(strings.HasPrefix(dev1, "/dev/"), Equals, true)
	stat1, err := os.Stat(dev1)
	c.Assert(err, IsNil)
	c.Assert(stat1.Mode()&os.ModeDevice != 0, Equals, true)
	log.Debug("Attached volume1 at ", dev1)

	devMap, err = svc.getInstanceDevList()
	c.Assert(err, IsNil)
	c.Assert(len(devMap), Equals, originDevCounts+1)

	log.Debug("Creating snapshot1")
	rs1Tags := map[string]string{
		"Name": "snapshot1",
	}
	rs1 := &CreateSnapshotRequest{
		VolumeID:    volumeID1,
		Description: "Test snapshot",
		Tags:        rs1Tags,
	}
	snapshotID, err := svc.CreateSnapshot(rs1)
	c.Assert(err, IsNil)
	c.Assert(snapshotID, Not(Equals), "")
	log.Debug("Waiting for snapshot1 complete ", snapshotID)
	err = svc.WaitForSnapshotComplete(snapshotID)
	c.Assert(err, IsNil)
	tags, err = svc.GetTags(snapshotID)
	c.Assert(err, IsNil)
	c.Assert(rs1Tags, DeepEquals, tags)

	log.Debug("Creating gp2 type volume2 from snapshot1")
	r2 := &CreateEBSVolumeRequest{
		Size:       2 * GB,
		SnapshotID: snapshotID,
		VolumeType: "gp2",
	}
	volumeID2, err := svc.CreateVolume(r2)
	c.Assert(err, IsNil)
	c.Assert(volumeID2, Not(Equals), "")

	log.Debug("Copying snapshot1 to snapshot2")
	snapshotID2, err := svc.CopySnapshot(snapshotID, svc.Region)
	c.Assert(err, IsNil)
	c.Assert(snapshotID2, Not(Equals), "")
	log.Debug("Waiting for snapshot2 complete ", snapshotID2)
	err = svc.WaitForSnapshotComplete(snapshotID2)
	c.Assert(err, IsNil)

	log.Debug("Creating io1 type volume3 from snapshot2")
	r3 := &CreateEBSVolumeRequest{
		Size:       5 * GB,
		SnapshotID: snapshotID2,
		VolumeType: "io1",
		IOPS:       100,
	}
	volumeID3, err := svc.CreateVolume(r3)
	c.Assert(err, IsNil)
	c.Assert(volumeID3, Not(Equals), "")

	log.Debug("Deleting snapshot1")
	err = svc.DeleteSnapshot(snapshotID)
	c.Assert(err, IsNil)

	log.Debug("Deleting snapshot2")
	err = svc.DeleteSnapshot(snapshotID2)
	c.Assert(err, IsNil)

	log.Debug("Deleting volume3")
	err = svc.DeleteVolume(volumeID3)
	c.Assert(err, IsNil)

	log.Debug("Attaching volume2")
	dev2, err := svc.AttachVolume(volumeID2, 2*GB)
	c.Assert(err, IsNil)
	c.Assert(strings.HasPrefix(dev2, "/dev/"), Equals, true)
	stat2, err := os.Stat(dev2)
	c.Assert(err, IsNil)
	c.Assert(stat2.Mode()&os.ModeDevice != 0, Equals, true)
	log.Debug("Attached volume2 at ", dev2)

	devMap, err = svc.getInstanceDevList()
	c.Assert(err, IsNil)
	c.Assert(len(devMap), Equals, originDevCounts+2)

	log.Debug("Detaching volume2")
	err = svc.DetachVolume(volumeID2)
	c.Assert(err, IsNil)

	log.Debug("Deleting volume2")
	err = svc.DeleteVolume(volumeID2)
	c.Assert(err, IsNil)

	devMap, err = svc.getInstanceDevList()
	c.Assert(err, IsNil)
	c.Assert(len(devMap), Equals, originDevCounts+1)

	log.Debug("Detaching volume1")
	err = svc.DetachVolume(volumeID1)
	c.Assert(err, IsNil)

	devMap, err = svc.getInstanceDevList()
	c.Assert(err, IsNil)
	c.Assert(len(devMap), Equals, originDevCounts)

	log.Debug("Deleting volume1")
	err = svc.DeleteVolume(volumeID1)
	c.Assert(err, IsNil)
}
