// +build linux,devmapper

package devmapper

import (
	"code.google.com/p/go-uuid/uuid"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/storagedriver"
	"github.com/rancher/rancher-volume/util"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	. "gopkg.in/check.v1"
)

const (
	dataFile      = "data.vol"
	metadataFile  = "metadata.vol"
	imageFile     = "test.img"
	imageTestFile = "image.exists"
	poolName      = "test_pool"
	devRoot       = "/tmp/devmapper"
	devDataRoot   = "/tmp/devmapper/data"
	devCfgRoot    = "/tmp/devmapper/cfg"
	devMount      = "/tmp/devmapper/mount"
	volumeSize    = 1 << 26
	dataSize      = 1 << 30
	metadataSize  = 1 << 28
	maxThin       = 10000
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	dataDev      string
	dataFile     string
	metadataDev  string
	metadataFile string
	imageFile    string
	driver       storagedriver.StorageDriver
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)

	var err error

	err = exec.Command("mkdir", "-p", devRoot).Run()
	c.Assert(err, IsNil)

	err = exec.Command("mkdir", "-p", devMount).Run()
	c.Assert(err, IsNil)

	// Prepare base image
	s.imageFile = filepath.Join(devRoot, imageFile)
	err = exec.Command("truncate", "-s", strconv.Itoa(volumeSize), s.imageFile).Run()
	c.Assert(err, IsNil)

	tmpDev, err := util.AttachLoopbackDevice(s.imageFile, false)
	c.Assert(err, IsNil)

	err = exec.Command("mkfs", "-t", "ext4", tmpDev).Run()
	c.Assert(err, IsNil)

	err = exec.Command("mount", tmpDev, devMount).Run()
	c.Assert(err, IsNil)

	err = exec.Command("touch", filepath.Join(devMount, imageTestFile)).Run()
	c.Assert(err, IsNil)

	err = exec.Command("umount", devMount).Run()
	c.Assert(err, IsNil)

	err = util.DetachLoopbackDevice(s.imageFile, tmpDev)
	c.Assert(err, IsNil)
}

func (s *TestSuite) SetUpTest(c *C) {
	s.driver = nil

	err := exec.Command("mkdir", "-p", devCfgRoot).Run()
	c.Assert(err, IsNil)

	err = exec.Command("mkdir", "-p", devDataRoot).Run()
	c.Assert(err, IsNil)

	s.dataFile = filepath.Join(devDataRoot, dataFile)
	s.metadataFile = filepath.Join(devDataRoot, metadataFile)

	err = exec.Command("truncate", "-s", strconv.Itoa(dataSize), s.dataFile).Run()
	c.Assert(err, IsNil)

	err = exec.Command("truncate", "-s", strconv.Itoa(metadataSize), s.metadataFile).Run()
	c.Assert(err, IsNil)

	s.dataDev, err = util.AttachLoopbackDevice(s.dataFile, false)
	c.Assert(err, IsNil)

	s.metadataDev, err = util.AttachLoopbackDevice(s.metadataFile, false)
	c.Assert(err, IsNil)

	s.initDriver(c)
}

func (s *TestSuite) TearDownTest(c *C) {
	var err error

	err = exec.Command("dmsetup", "remove", "--retry", poolName).Run()
	c.Check(err, IsNil)

	err = exec.Command("losetup", "-d", s.dataDev, s.metadataDev).Run()
	c.Check(err, IsNil)

	err = exec.Command("rm", "-rf", devCfgRoot).Run()
	c.Check(err, IsNil)

	err = exec.Command("rm", "-rf", devDataRoot).Run()
	c.Check(err, IsNil)
}

func (s *TestSuite) TearDownSuite(c *C) {
	err := exec.Command("rm", "-rf", devRoot).Run()
	c.Assert(err, IsNil)
}

func (s *TestSuite) initDriver(c *C) {
	config := make(map[string]string)

	_, err := Init(devCfgRoot, config)
	c.Assert(err, ErrorMatches, "data device or metadata device unspecified")

	config[DM_DATA_DEV] = s.dataDev
	config[DM_METADATA_DEV] = s.metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devCfgRoot, config)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Block size must.*")

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devCfgRoot, config)
	c.Assert(err, IsNil)

	newDriver, err := Init(devCfgRoot, config)
	c.Assert(err, IsNil)

	drv1, ok := driver.(*Driver)
	c.Assert(ok, Equals, true)
	drv2, ok := newDriver.(*Driver)
	c.Assert(ok, Equals, true)

	c.Assert(*drv1, DeepEquals, *drv2)

	c.Assert(drv1.DataDevice, Equals, s.dataDev)
	c.Assert(drv1.MetadataDevice, Equals, s.metadataDev)

	s.driver = driver
}

func (s *TestSuite) TestVolume(c *C) {
	var err error
	driver := s.driver

	drv := driver.(*Driver)
	lastDevID := drv.LastDevID
	volumeID := uuid.New()

	volOps, err := driver.VolumeOps()
	c.Assert(err, IsNil)

	opts := map[string]string{
		storagedriver.OPT_SIZE: strconv.FormatInt(volumeSize, 10),
	}
	err = volOps.CreateVolume(volumeID, opts)
	c.Assert(err, IsNil)

	c.Assert(drv.LastDevID, Equals, lastDevID+1)

	err = volOps.CreateVolume(volumeID, opts)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Already has volume with specific uuid.*")

	volumeID2 := uuid.New()

	wrongOpts := map[string]string{
		storagedriver.OPT_SIZE: "1333333",
	}
	err = volOps.CreateVolume(volumeID2, wrongOpts)
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "Size must be multiple of block size")

	err = volOps.CreateVolume(volumeID2, opts)
	c.Assert(err, IsNil)

	listOpts := map[string]string{
		storagedriver.OPT_VOLUME_UUID: volumeID,
	}
	_, err = volOps.ListVolume(map[string]string{})
	c.Assert(err, IsNil)

	_, err = volOps.ListVolume(listOpts)
	c.Assert(err, IsNil)

	err = volOps.DeleteVolume("123")
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Cannot find object .*")

	err = volOps.DeleteVolume(volumeID2)
	c.Assert(err, IsNil)

	err = volOps.DeleteVolume(volumeID)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestSnapshot(c *C) {
	var err error
	driver := s.driver

	volOps, err := driver.VolumeOps()
	c.Assert(err, IsNil)
	snapOps, err := driver.SnapshotOps()
	c.Assert(err, IsNil)

	volumeID := uuid.New()
	opts := map[string]string{
		storagedriver.OPT_SIZE: strconv.FormatInt(volumeSize, 10),
	}
	err = volOps.CreateVolume(volumeID, opts)
	c.Assert(err, IsNil)

	snapshotID := uuid.New()
	err = snapOps.CreateSnapshot(snapshotID, volumeID)
	c.Assert(err, IsNil)

	err = snapOps.CreateSnapshot(snapshotID, volumeID)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Already has snapshot with uuid.*")

	snapshotID2 := uuid.New()
	err = snapOps.CreateSnapshot(snapshotID2, volumeID)
	c.Assert(err, IsNil)

	err = snapOps.DeleteSnapshot(snapshotID, volumeID)
	c.Assert(err, IsNil)

	err = snapOps.DeleteSnapshot(snapshotID, volumeID)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "cannot find snapshot .*")

	err = volOps.DeleteVolume(volumeID)
	c.Assert(err, IsNil)
}
