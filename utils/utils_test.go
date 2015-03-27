package utils

import (
	"reflect"
	"strings"
	"testing"
)

type Device struct {
	Root              string
	DataDevice        string
	MetadataDevice    string
	ThinpoolDevice    string
	ThinpoolSize      uint64
	ThinpoolBlockSize uint32
	Volumes           map[string]Volume
}

type Volume struct {
	DevID int
	Size  uint64
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

	dev.Volumes = make(map[string]Volume)
	err := SaveConfig("/tmp/cfg", &dev)
	if err != nil {
		t.Fatal("Fail to save config!", err)
	}

	dev.ThinpoolBlockSize = 2048

	volume := Volume{
		DevID: 1,
		Size:  1000000,
	}
	dev.Volumes["123"] = volume

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

func TestLockFile(t *testing.T) {
	file := "/tmp/t.lock"
	if err := LockFile(file); err != nil {
		t.Fatal("Failed to unlock the file!", err)
	}
	if err := LockFile(file); err == nil || strings.HasPrefix(err.Error(), "resource tempoarily unavailable") {
		t.Fatal("Shouldn't allow double lock file!", err)
	}
	if err := LockFile(file); err == nil || strings.HasPrefix(err.Error(), "resource tempoarily unavailable") {
		t.Fatal("Shouldn't allow double lock file!", err)
	}
	if err := UnlockFile(file); err != nil {
		t.Fatal("Failed to unlock the file!", err)
	}

	if err := LockFile(file); err != nil {
		t.Fatal("Failed to unlock the file!", err)
	}
	if err := UnlockFile(file); err != nil {
		t.Fatal("Failed to unlock the file!", err)
	}
}
