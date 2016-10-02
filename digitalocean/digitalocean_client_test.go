//+build dotest

package digitalocean

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/cloudflare/cfssl/log"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct{}

var _ = Suite(&TestSuite{})

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)
}

func (s *TestSuite) TestDOMetadata(c *C) {
	svc, err := NewClient()
	c.Assert(err, IsNil)

	c.Assert(svc.region, Not(Equals), "")
	c.Assert(svc.id, Not(Equals), 0)
}

func (s *TestSuite) TestVolume(c *C) {
	svc, err := NewClient()
	c.Assert(err, IsNil)

	log.Debug("creating volume")
	id, err := svc.CreateVolume("volume1", GB)
	c.Assert(err, IsNil)
	c.Assert(id, Not(Equals), "")

	log.Debug("attaching volume")
	err = svc.AttachVolume(id)
	c.Assert(err, IsNil)

	dev := filepath.Join(DO_DEVICE_FOLDER, DO_DEVICE_PREFIX+"volume1")
	stat1, err := os.Stat(dev)
	c.Assert(err, IsNil)
	c.Assert(stat1.Mode()&os.ModeDevice != 0, Equals, true)

	log.Debug("detaching & deleting volume")
	err = svc.DetachVolume(id)
	c.Assert(err, IsNil)

	err = svc.DeleteVolume(id)
	c.Assert(err, IsNil)
}
