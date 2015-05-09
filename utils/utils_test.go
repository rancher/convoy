package utils

import (
	"code.google.com/p/go-uuid/uuid"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
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
	err := SaveConfig("/tmp", "cfg", &dev)
	if err != nil {
		t.Fatal("Fail to save config!", err)
	}

	dev.ThinpoolBlockSize = 2048

	volume := Volume{
		DevID: 1,
		Size:  1000000,
	}
	dev.Volumes["123"] = volume

	err = SaveConfig("/tmp", "cfg", &dev)
	if err != nil {
		t.Fatal("Fail to update config!", err)
	}

	devNew := Device{}
	err = LoadConfig("/tmp", "cfg", &devNew)
	if err != nil {
		t.Fatal("Fail to load config!", err)
	}

	if !reflect.DeepEqual(dev, devNew) {
		t.Fatal("Fail to complete save/load config correctly!")
	}
}

func TestListConfigIDs(t *testing.T) {
	tmpdir, err := ioutil.TempDir("/tmp", "volmgr")
	if err != nil {
		t.Fatal("Fail to get temp dir")
	}
	defer os.RemoveAll(tmpdir)

	prefix := "prefix_"
	suffix := "_suffix.cfg"
	ids := ListConfigIDs(tmpdir, prefix, suffix)
	if len(ids) != 0 {
		t.Fatal("Files out of nowhere! IDs", ids)
	}
	counts := 10
	uuids := make(map[string]bool)
	for i := 0; i < counts; i++ {
		id := uuid.New()
		uuids[id] = true
		if err := exec.Command("touch", filepath.Join(tmpdir, prefix+id+suffix)).Run(); err != nil {
			t.Fatal("Fail to create test files")
		}
	}
	uuidList := ListConfigIDs(tmpdir, prefix, suffix)
	if len(uuidList) != counts {
		t.Fatal("Wrong result for list")
	}
	for i := 0; i < counts; i++ {
		if _, exists := uuids[uuidList[i]]; !exists {
			t.Fatal("Wrong key for list")
		}
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

func TestSliceToMap(t *testing.T) {
	legalMap := []string{
		"a=1",
		"b=2",
	}
	m := SliceToMap(legalMap)
	if m["a"] != "1" || m["b"] != "2" {
		t.Fatal("Failed test, result is not expected!")
	}
	illegalMap := []string{
		"a=1",
		"bcd",
	}
	m = SliceToMap(illegalMap)
	if m != nil {
		t.Fatal("Failed illegal test!")
	}

}
