// +build linux,devmapper

package devmapper

import (
	"code.google.com/p/go-uuid/uuid"
	log "github.com/Sirupsen/logrus"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "gopkg.in/check.v1"
)

const (
	dataFile     = "data.vol"
	metadataFile = "metadata.vol"
	poolName     = "test_pool"
	devRoot      = "/tmp/devmapper"
	devCfg       = "driver_devicemapper.cfg"
	volumeSize   = 1 << 27
	maxThin      = 10000
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	dataDev     string
	metadataDev string
}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stderr)

	var err error

	err = exec.Command("mkdir", "-p", devRoot).Run()
	c.Assert(err, IsNil)

	err = exec.Command("dd", "if=/dev/zero", "of="+filepath.Join(devRoot, dataFile), "bs=4096", "count=262114").Run()
	c.Assert(err, IsNil)

	err = exec.Command("dd", "if=/dev/zero", "of="+filepath.Join(devRoot, metadataFile), "bs=4096", "count=10000").Run()
	c.Assert(err, IsNil)

	out, err := exec.Command("losetup", "-v", "-f", filepath.Join(devRoot, dataFile)).Output()
	c.Assert(err, IsNil)

	s.dataDev = strings.TrimSpace(strings.SplitAfter(string(out[:]), "device is")[1])

	out, err = exec.Command("losetup", "-v", "-f", filepath.Join(devRoot, metadataFile)).Output()
	c.Assert(err, IsNil)
	s.metadataDev = strings.TrimSpace(strings.SplitAfter(string(out[:]), "device is")[1])
}

func (s *TestSuite) TearDownSuite(c *C) {
	var err error

	err = exec.Command("dmsetup", "remove", poolName).Run()
	c.Assert(err, IsNil)

	err = exec.Command("losetup", "-d", s.dataDev, s.metadataDev).Run()
	c.Assert(err, IsNil)

	err = exec.Command("rm", "-rf", devRoot).Run()
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestInit(c *C) {
	config := make(map[string]string)

	_, err := Init(devRoot, devCfg, config)
	c.Assert(err, ErrorMatches, "data device or metadata device unspecified")

	config[DM_DATA_DEV] = s.dataDev
	config[DM_METADATA_DEV] = s.metadataDev
	config[DM_THINPOOL_BLOCK_SIZE] = "100"
	_, err = Init(devRoot, devCfg, config)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "Block size must.*")

	config[DM_THINPOOL_NAME] = "test_pool"
	delete(config, DM_THINPOOL_BLOCK_SIZE)

	driver, err := Init(devRoot, devCfg, config)
	c.Assert(err, IsNil)

	newDriver, err := Init(devRoot, devCfg, config)
	c.Assert(err, IsNil)

	drv1, ok := driver.(*Driver)
	c.Assert(ok, Equals, true)
	drv2, ok := newDriver.(*Driver)
	c.Assert(ok, Equals, true)

	c.Assert(*drv1, DeepEquals, *drv2)

	c.Assert(drv1.configName, Equals, devCfg)

	c.Assert(drv1.DataDevice, Equals, s.dataDev)
	c.Assert(drv1.MetadataDevice, Equals, s.metadataDev)
}

func (s *TestSuite) TestVolume(c *C) {
	driver, err := Init(devRoot, devCfg, nil)
	c.Assert(err, IsNil)

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
	driver, err := Init(devRoot, devCfg, nil)
	c.Assert(err, IsNil)

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
