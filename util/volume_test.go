package util

import (
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"
)

type HelperVolume struct {
	UUID       string
	Device     string
	MountPoint string
}

const (
	testMountPath = "/tmp/util/mnt"
)

func (v *HelperVolume) GetDevice() (string, error) {
	return v.Device, nil
}

func (v *HelperVolume) GetMountOpts() []string {
	return []string{}
}

func (v *HelperVolume) GenerateDefaultMountPoint() string {
	return filepath.Join(testMountPath, v.UUID)
}

func (s *TestSuite) TestVolumeHelper(c *C) {
	dev, err := AttachLoopbackDevice(s.imageFile, true)
	c.Assert(err, IsNil)

	r := &HelperVolume{
		UUID:   "testabc",
		Device: dev,
	}

	m, err := VolumeMount(r, "")
	c.Assert(err, IsNil)
	c.Assert(strings.HasPrefix(m, testMountPath), Equals, true)
	c.Assert(r.MountPoint, Equals, m)

	m2, err := VolumeMount(r, "")
	c.Assert(err, IsNil)
	c.Assert(m2, Equals, m)

	newMountPoint := "/tmp/util/mnt/test"
	_, err = VolumeMount(r, newMountPoint)
	c.Assert(err, ErrorMatches, "Specified mount point "+newMountPoint+" is not a directory")

	err = MkdirIfNotExists(newMountPoint)
	c.Assert(err, IsNil)
	_, err = VolumeMount(r, newMountPoint)
	c.Assert(err, ErrorMatches, "Volume "+r.UUID+" was already mounted at "+r.MountPoint+".*")

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	m, err = VolumeMount(r, newMountPoint)
	c.Assert(err, IsNil)
	c.Assert(m, Equals, newMountPoint)
	c.Assert(r.MountPoint, Equals, newMountPoint)

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	err = DetachLoopbackDevice(s.imageFile, dev)
	c.Assert(err, IsNil)
}
