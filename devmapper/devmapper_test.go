// +build linux,devmapper

package devmapper

import (
	"code.google.com/p/go-uuid/uuid"
	log "github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/util"
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
	devCfg        = "driver_devicemapper.cfg"
	volumeSize    = 1 << 26
	dataSize      = 1 << 28
	metadataSize  = 1 << 25
	maxThin       = 10000
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	dataDev      string
	dataFile     string
	metadataDev  string
	metadataFile string
	imageFile    string
	driver       drivers.Driver
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)

	var err error

	err = exec.Command("mkdir", "-p", devRoot).Run()
	c.Assert(err, IsNil)

	err = exec.Command("mkdir", "-p", devMount).Run()
	c.Assert(err, IsNil)

	// Prepare base image
	s.imageFile = filepath.Join(devRoot, imageFile)
	err = exec.Command("dd", "if=/dev/zero", "of="+s.imageFile, "bs=4096",
		"count="+strconv.Itoa(volumeSize/4096)).Run()
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

	err = exec.Command("dd", "if=/dev/zero", "of="+s.dataFile, "bs=4096",
		"count="+strconv.Itoa(dataSize/4096)).Run()
	c.Assert(err, IsNil)

	err = exec.Command("dd", "if=/dev/zero", "of="+s.metadataFile, "bs=4096",
		"count="+strconv.Itoa(metadataSize/4096)).Run()
	c.Assert(err, IsNil)

	s.dataDev, err = util.AttachLoopbackDevice(s.dataFile, false)
	c.Assert(err, IsNil)

	s.metadataDev, err = util.AttachLoopbackDevice(s.metadataFile, false)
	c.Assert(err, IsNil)

	s.initDriver(c)
}

func (s *TestSuite) TearDownTest(c *C) {
	var err error

	err = exec.Command("dmsetup", "remove", poolName).Run()
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

	_, err := Init(devCfgRoot, devCfg, config)
	c.Assert(err, ErrorMatches, "data device or metadata device unspecified")

	config[DM_DATA_DEV] = s.dataDev
	config[DM_METADATA_DEV] = s.metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devCfgRoot, devCfg, config)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Block size must.*")

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devCfgRoot, devCfg, config)
	c.Assert(err, IsNil)

	newDriver, err := Init(devCfgRoot, devCfg, config)
	c.Assert(err, IsNil)

	drv1, ok := driver.(*Driver)
	c.Assert(ok, Equals, true)
	drv2, ok := newDriver.(*Driver)
	c.Assert(ok, Equals, true)

	c.Assert(*drv1, DeepEquals, *drv2)

	c.Assert(drv1.configName, Equals, devCfg)

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

	err = driver.CreateVolume(volumeID, "", volumeSize)
	c.Assert(err, IsNil)

	c.Assert(drv.LastDevID, Equals, lastDevID+1)

	err = driver.CreateVolume(volumeID, "", volumeSize)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Already has volume with uuid.*")

	volumeID2 := uuid.New()

	wrongVolumeSize := int64(13333333)
	err = driver.CreateVolume(volumeID2, "", wrongVolumeSize)
	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "Size must be multiple of block size")

	err = driver.CreateVolume(volumeID2, "", volumeSize)
	c.Assert(err, IsNil)

	err = driver.ListVolume("", "")
	c.Assert(err, IsNil)

	err = driver.ListVolume(volumeID, "")
	c.Assert(err, IsNil)

	err = driver.DeleteVolume("123")
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "cannot find volume.*")

	err = driver.DeleteVolume(volumeID2)
	c.Assert(err, IsNil)

	err = driver.DeleteVolume(volumeID)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestSnapshot(c *C) {
	var err error
	driver := s.driver

	volumeID := uuid.New()
	err = driver.CreateVolume(volumeID, "", volumeSize)
	c.Assert(err, IsNil)

	snapshotID := uuid.New()
	err = driver.CreateSnapshot(snapshotID, volumeID)
	c.Assert(err, IsNil)

	err = driver.CreateSnapshot(snapshotID, volumeID)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Already has snapshot with uuid.*")

	snapshotID2 := uuid.New()
	err = driver.CreateSnapshot(snapshotID2, volumeID)
	c.Assert(err, IsNil)

	err = driver.DeleteSnapshot(snapshotID, volumeID)
	c.Assert(err, IsNil)

	err = driver.DeleteSnapshot(snapshotID, volumeID)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "cannot find snapshot.*")

	err = driver.DeleteVolume(volumeID)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, ".*delete snapshots first")

	err = driver.DeleteSnapshot(snapshotID2, volumeID)
	c.Assert(err, IsNil)

	err = driver.DeleteVolume(volumeID)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestImage(c *C) {
	var err error
	driver := s.driver

	imageID := uuid.New()
	err = driver.ActivateImage(imageID, s.imageFile)
	c.Assert(err, IsNil)

	err = driver.DeactivateImage(imageID)
	c.Assert(err, IsNil)

	err = driver.ActivateImage(imageID, s.imageFile)
	c.Assert(err, IsNil)

	err = driver.ActivateImage(imageID, s.imageFile)
	c.Assert(err, ErrorMatches, ".*already activated.*")

	err = driver.DeactivateImage(imageID)
	c.Assert(err, IsNil)

	err = driver.DeactivateImage(imageID)
	c.Assert(err, ErrorMatches, "Cannot find image.*")
}

func (s *TestSuite) TestCreateVolumeWithBaseImage(c *C) {
	var err error
	driver := s.driver

	imageID := uuid.New()
	err = driver.ActivateImage(imageID, s.imageFile)
	c.Assert(err, IsNil)

	volumeID := uuid.New()
	err = driver.CreateVolume(volumeID, imageID, volumeSize)
	c.Assert(err, IsNil)

	err = drivers.Mount(driver, volumeID, devMount, "ext4", "", false, "")
	c.Assert(err, IsNil)

	_, err = os.Stat(filepath.Join(devMount, imageTestFile))
	c.Assert(err, IsNil)

	err = drivers.Unmount(driver, devMount, "")
	c.Assert(err, IsNil)

	err = driver.DeleteVolume(volumeID)
	c.Assert(err, IsNil)

	err = driver.DeactivateImage(imageID)
	c.Assert(err, IsNil)
}
