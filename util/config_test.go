package util

import (
	"fmt"
	"path/filepath"

	. "gopkg.in/check.v1"
)

type Device struct {
	UUID              string
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      uint64
	ThinpoolBlockSize uint32
	Volumes           map[string]Volume
}

type Volume struct {
	ID    string
	DevID int
	Size  uint64
}

type RandomStruct struct {
	Field string
}

func (s *TestSuite) TestSaveLoadConfig(c *C) {
	dev := Device{
		Root:              "/tmp/convoy/devmapper",
		DataDevice:        "/dev/loop0",
		MetadataDevice:    "/dev/loop1",
		ThinpoolDevice:    "/dev/mapper/convoy-pool",
		ThinpoolSize:      1024 * 1024 * 1024,
		ThinpoolBlockSize: 4096,
	}

	dev.Volumes = make(map[string]Volume)
	err := SaveConfig("/tmp/cfg", &dev)
	c.Assert(err, IsNil)

	dev.ThinpoolBlockSize = 2048

	volume := Volume{
		DevID: 1,
		Size:  1000000,
	}
	dev.Volumes["123"] = volume

	err = SaveConfig("/tmp/cfg", &dev)
	c.Assert(err, IsNil)

	devNew := Device{}
	err = LoadConfig("/tmp/cfg", &devNew)
	c.Assert(err, IsNil)

	c.Assert(dev, DeepEquals, devNew)
}

func (d *Device) ConfigFile() (string, error) {
	return filepath.Join(testRoot, "device.cfg"), nil
}

func (v *Volume) ConfigFile() (string, error) {
	if v.ID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume ID")
	}
	return filepath.Join(testRoot, "volume-"+v.ID+".cfg"), nil
}

func (s *TestSuite) TestSaveLoadObject(c *C) {
	var err error
	dev := &Device{
		Root:              "/tmp/convoy/devmapper",
		DataDevice:        "/dev/loop0",
		MetadataDevice:    "/dev/loop1",
		ThinpoolDevice:    "/dev/mapper/convoy-pool",
		ThinpoolSize:      1024 * 1024 * 1024,
		ThinpoolBlockSize: 4096,
	}
	dev.Volumes = make(map[string]Volume)
	vol1 := &Volume{
		ID:    "123",
		DevID: 1,
		Size:  1000000,
	}
	vol2 := &Volume{
		ID:    "456",
		DevID: 2,
		Size:  2000000,
	}

	dev.Volumes[vol1.ID] = *vol1
	dev.Volumes[vol2.ID] = *vol2

	// Sanity test
	ops, err := getObjectOps(dev)
	c.Assert(err, IsNil)
	c.Assert(ops, DeepEquals, dev)

	r := &RandomStruct{}
	ops, err = getObjectOps(*r)
	c.Assert(err, ErrorMatches, "BUG: Non-pointer was passed in")

	ops, err = getObjectOps(r)
	c.Assert(err, ErrorMatches, "BUG: util.RandomStruct doesn't implement.*")

	// test without ID
	exists, err := ObjectExists(&Device{})
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = ObjectSave(dev)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(&Device{})
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	d1 := &Device{}
	err = ObjectLoad(d1)
	c.Assert(err, IsNil)
	c.Assert(dev, DeepEquals, d1)

	err = ObjectDelete(d1)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(&Device{})
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = ObjectLoad(d1)
	c.Assert(err, ErrorMatches, "No such volume.*")

	// test with ID
	exists, err = ObjectExists(&Volume{})
	c.Assert(err, ErrorMatches, "BUG: Invalid empty volume ID")

	vol := &Volume{}
	err = ObjectLoad(vol)
	c.Assert(err, ErrorMatches, "BUG: Invalid empty volume ID")

	exists, err = ObjectExists(vol1)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = ObjectSave(vol1)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(vol1)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	exists, err = ObjectExists(vol2)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = ObjectSave(vol2)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(vol2)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	vol.ID = "123"
	err = ObjectLoad(vol)
	c.Assert(err, IsNil)
	c.Assert(vol1, DeepEquals, vol)

	vol.ID = "456"
	err = ObjectLoad(vol)
	c.Assert(err, IsNil)
	c.Assert(vol2, DeepEquals, vol)

	err = ObjectDelete(vol1)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(vol1)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	exists, err = ObjectExists(vol2)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	err = ObjectDelete(vol2)
	c.Assert(err, IsNil)

	exists, err = ObjectExists(vol2)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

}
