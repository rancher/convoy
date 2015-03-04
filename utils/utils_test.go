package utils

import (
	"reflect"
	"testing"
)

type Device struct {
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      uint64
	ThinpoolBlockSize uint32
}

func TestSaveLoadConfig(t *testing.T) {
	dev := Device{
		Root:              "/tmp/volmgr/devmapper",
		DataDevice:        "/dev/loop0",
		MetadataDevice:    "/dev/loop1",
		ThinpoolDevice:    "/dev/mapper/rancher-volume-pool",
		ThinpoolSize:      1024 * 1024 * 1024,
		ThinpoolBlockSize: 4096,
	}

	err := SaveConfig("/tmp/cfg", &dev)
	if err != nil {
		t.Fatal("Fail to save config!", err)
	}

	dev.ThinpoolBlockSize = 2048
	err = SaveConfig("/tmp/cfg", &dev)
	if err != nil {
		t.Fatal("Fail to update config!", err)
	}

	devNew := Device{}
	err = LoadConfig("/tmp/cfg", &devNew)
	if err != nil {
		t.Fatal("Fail to load config!", err)
	}

	if !reflect.DeepEqual(dev, devNew) {
		t.Fatal("Fail to complete save/load config correctly!")
	}
}
