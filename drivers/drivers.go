package drivers

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancherio/volmgr/metadata"
	"os/exec"

	. "github.com/rancherio/volmgr/logging"
)

type InitFunc func(root, cfgName string, config map[string]string) (Driver, error)

type Driver interface {
	Name() string
	CreateVolume(id, baseID string, size int64) error
	DeleteVolume(id string) error
	GetVolumeDevice(id string) (string, error)
	ListVolume(id, snapshotID string) error
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
	Info() error
	ActivateImage(imageUUID, imageFile string) error
	DeactivateImage(imageUUID string) error
}

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "drivers"})
)

func init() {
	initializers = make(map[string]InitFunc)
}

func Register(name string, initFunc InitFunc) error {
	if _, exists := initializers[name]; exists {
		return fmt.Errorf("Driver %s has already been registered", name)
	}
	initializers[name] = initFunc
	return nil
}

func getCfgName(name string) string {
	return "driver_" + name + ".cfg"
}

func GetDriver(name, root string, config map[string]string) (Driver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	return initializers[name](root, getCfgName(name), config)
}

func Mount(driver Driver, volumeUUID, mountPoint, fstype, option string, needFormat bool, newNS string) error {
	dev, err := driver.GetVolumeDevice(volumeUUID)
	if err != nil {
		return err
	}
	if fstype != "ext4" {
		return fmt.Errorf("unsupported filesystem ", fstype)
	}
	if needFormat {
		if err := exec.Command("mkfs."+fstype, dev).Run(); err != nil {
			return err
		}
	}
	if newNS == "" {
		newNS = "/proc/1/ns/mnt"
	}
	cmdline := []string{newNS, "-m", "-t", fstype}
	if option != "" {
		cmdline = append(cmdline, option)
	}
	cmdline = append(cmdline, dev, mountPoint)
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_MOUNT,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_MOUNTPOINT: mountPoint,
		LOG_FIELD_OPTION:     cmdline,
	}).Debug()
	output, err := exec.Command("volmgr_mount", cmdline...).CombinedOutput()
	if err != nil {
		log.Error("Failed mount, ", string(output))
		return err
	}
	return nil
}

func Unmount(driver Driver, mountPoint, newNS string) error {
	if newNS == "" {
		newNS = "/proc/1/ns/mnt"
	}
	cmdline := []string{newNS, "-u", mountPoint}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_UMOUNT,
		LOG_FIELD_MOUNTPOINT: mountPoint,
		LOG_FIELD_OPTION:     cmdline,
	}).Debug()
	output, err := exec.Command("volmgr_mount", cmdline...).CombinedOutput()
	if err != nil {
		log.Error("Failed umount, ", string(output))
		return err
	}
	return nil
}
