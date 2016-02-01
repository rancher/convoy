package util

import (
	"path/filepath"
	"strings"

	. "gopkg.in/check.v1"
)

type HelperVolume struct {
	Name       string
	Device     string
	MountPoint string
}

const (
	testMountPath = "/tmp/util/mnt"
	devImage      = "dev.img"
)

func (v *HelperVolume) GetDevice() (string, error) {
	return v.Device, nil
}

func (v *HelperVolume) GetMountOpts() []string {
	return []string{}
}

func (v *HelperVolume) GenerateDefaultMountPoint() string {
	return filepath.Join(testMountPath, v.Name)
}

func (s *TestSuite) TestVolumeHelper(c *C) {
	dev, err := AttachLoopbackDevice(s.imageFile, false)
	c.Assert(err, IsNil)

	r := &HelperVolume{
		Name:   "testabc",
		Device: dev,
	}

	m, err := VolumeMount(r, "", false)
	c.Assert(err, IsNil)
	c.Assert(strings.HasPrefix(m, testMountPath), Equals, true)
	c.Assert(r.MountPoint, Equals, m)

	m2, err := VolumeMount(r, "", false)
	c.Assert(err, IsNil)
	c.Assert(m2, Equals, m)

	newMountPoint := "/tmp/util/mnt"
	_, err = VolumeMount(r, newMountPoint, false)
	c.Assert(err, ErrorMatches, "Volume "+r.Name+" was already mounted at "+r.MountPoint+".*")

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	m, err = VolumeMount(r, newMountPoint, false)
	c.Assert(err, IsNil)
	c.Assert(m, Equals, newMountPoint)
	c.Assert(r.MountPoint, Equals, newMountPoint)

	exists := VolumeMountPointFileExists(r, "test_dir", FILE_TYPE_DIRECTORY)
	c.Assert(exists, Equals, false)

	err = VolumeMountPointDirectoryCreate(r, "test_dir")
	c.Assert(err, IsNil)

	exists = VolumeMountPointFileExists(r, "test_dir", FILE_TYPE_DIRECTORY)
	c.Assert(exists, Equals, true)

	testVolumeVMSupport(r, s, c)

	err = VolumeMountPointDirectoryRemove(r, "test_dir")
	c.Assert(err, IsNil)

	exists = VolumeMountPointFileExists(r, "test_dir", FILE_TYPE_DIRECTORY)
	c.Assert(exists, Equals, false)

	err = VolumeUmount(r)
	c.Assert(err, IsNil)
	c.Assert(r.MountPoint, Equals, "")

	err = DetachLoopbackDevice(s.imageFile, dev)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestVolumeHelperWithNamespace(c *C) {
	InitMountNamespace("/proc/1/ns/mnt")
	s.TestVolumeHelper(c)
}

func testVolumeVMSupport(r *HelperVolume, s *TestSuite, c *C) {
	var err error

	// Test image file
	err = MountPointPrepareImageFile(r.MountPoint, imageSize)
	c.Assert(err, IsNil)

	exists := VolumeMountPointFileExists(r, IMAGE_FILE_NAME, FILE_TYPE_REGULAR)
	c.Assert(exists, Equals, true)

	imgFile := filepath.Join(r.MountPoint, IMAGE_FILE_NAME)
	size, err := getFileSize(imgFile)
	c.Assert(err, IsNil)
	c.Assert(size, Equals, imageSize)

	// Test image device
	devFile := filepath.Join(testRoot, devImage)
	err = s.createFile(devFile, imageSize)

	originDev, err := AttachLoopbackDevice(devFile, false)
	c.Assert(err, IsNil)

	err = MountPointPrepareBlockDevice(r.MountPoint, originDev)
	c.Assert(err, IsNil)

	diskDev := filepath.Join(r.MountPoint, BLOCK_DEV_NAME)
	fileType, err := getFileType(diskDev)
	c.Assert(err, IsNil)
	c.Assert(fileType, Equals, FILE_TYPE_BLOCKDEVICE)

	err = MountPointRemoveFile(diskDev)
	c.Assert(err, IsNil)

	err = DetachLoopbackDevice(devFile, originDev)
	c.Assert(err, IsNil)

}
