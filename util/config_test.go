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

type RandomStruct1 struct {
	Field string
}

type RandomStruct2 struct {
	Field string
}

func (r *RandomStruct2) ConfigFile(id string) (string, error) {
	return "", nil
}

func (r *RandomStruct2) IdField() string {
	return "ID"
}

func (s *TestSuite) TestSaveLoadConfig(c *C) {
	dev := Device{
		Root:              "/tmp/rancher-volume/devmapper",
		DataDevice:        "/dev/loop0",
		MetadataDevice:    "/dev/loop1",
		ThinpoolDevice:    "/dev/mapper/rancher-volume-pool",
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

func (d *Device) ConfigFile(id string) (string, error) {
	if id != "" {
		return "", fmt.Errorf("Invalid ID %v specified for Device config", id)
	}
	return filepath.Join(testRoot, "device.cfg"), nil
}

func (d *Device) IdField() string {
	return ""
}

func (v *Volume) ConfigFile(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("Invalid empty ID specified for Volume config")
	}
	return filepath.Join(testRoot, "volume-"+id+".cfg"), nil
}

func (v *Volume) IdField() string {
	return "ID"
}

func (s *TestSuite) TestSaveLoadObject(c *C) {
	var err error
	dev := &Device{
		Root:              "/tmp/rancher-volume/devmapper",
		DataDevice:        "/dev/loop0",
		MetadataDevice:    "/dev/loop1",
		ThinpoolDevice:    "/dev/mapper/rancher-volume-pool",
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
	ops, id, err := getObjectOpts(dev)
	c.Assert(err, IsNil)
	c.Assert(ops, DeepEquals, dev)
	c.Assert(id, Equals, "")

	ops, id, err = getObjectOpts(vol1)
	c.Assert(err, IsNil)
	c.Assert(ops, DeepEquals, vol1)
	c.Assert(id, Equals, "123")

	ops, id, err = getObjectOpts(vol2)
	c.Assert(err, IsNil)
	c.Assert(ops, DeepEquals, vol2)
	c.Assert(id, Equals, "456")

	r1 := &RandomStruct1{}
	ops, id, err = getObjectOpts(*r1)
	c.Assert(err, ErrorMatches, "BUG: Non-pointer was passed in")

	ops, id, err = getObjectOpts(r1)
	c.Assert(err, ErrorMatches, "BUG: util.RandomStruct1 doesn't implement.*")

	r2 := &RandomStruct2{}
	ops, id, err = getObjectOpts(r2)
	c.Assert(err, ErrorMatches, "BUG: util.RandomStruct2 indicate ID field is ID.*")

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
	c.Assert(err, ErrorMatches, "Cannot find object config.*")

	// test with ID
	exists, err = ObjectExists(&Volume{})
	c.Assert(err, ErrorMatches, "Invalid empty ID.*")

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

	vol := &Volume{}
	err = ObjectLoad(vol)
	c.Assert(err, ErrorMatches, "Invalid empty ID.*")

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
