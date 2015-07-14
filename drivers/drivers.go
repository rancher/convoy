package drivers

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/metadata"
	"github.com/rancher/rancher-volume/util"

	. "github.com/rancher/rancher-volume/logging"
)

type InitFunc func(root, cfgName string, config map[string]string) (Driver, error)

type Driver interface {
	Name() string
	CreateVolume(id, baseID string, size int64) error
	DeleteVolume(id string) error
	GetVolumeDevice(id string) (string, error)
	ListVolume(id, snapshotID string) ([]byte, error)
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	HasSnapshot(id, volumeID string) bool
	CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error)
	OpenSnapshot(id, volumeID string) error
	ReadSnapshot(id, volumeID string, start int64, data []byte) error
	CloseSnapshot(id, volumeID string) error
	Info() ([]byte, error)
	ActivateImage(imageUUID, imageFile string) error
	DeactivateImage(imageUUID string) error
	Shutdown() error
	CheckEnvironment() error
}

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "drivers"})
)

const (
	RANCHER_MOUNT_BINARY      = "rancher-mount"
	RANCHER_MOUNT_MIN_VERSION = "0.1"
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

func Format(driver Driver, volumeUUID, fs string) error {
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_FORMAT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volumeUUID,
	}).Debug()
	dev, err := driver.GetVolumeDevice(volumeUUID)
	if err != nil {
		return err
	}
	if fs != "ext4" {
		return fmt.Errorf("unsupported filesystem ", fs)
	}
	if _, err := util.Execute("mkfs", []string{"-t", fs, dev}); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_FORMAT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volumeUUID,
	}).Debug()
	return nil
}

func Mount(driver Driver, volumeUUID, mountPoint, newNS string) error {
	dev, err := driver.GetVolumeDevice(volumeUUID)
	if err != nil {
		return err
	}
	fs := "ext4"

	if newNS == "" {
		newNS = "/proc/1/ns/mnt"
	}
	cmdline := []string{newNS, "-m", "-t", fs}
	cmdline = append(cmdline, dev, mountPoint)
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_START,
		LOG_FIELD_EVENT:      LOG_EVENT_MOUNT,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_MOUNTPOINT: mountPoint,
		LOG_FIELD_OPTION:     cmdline,
	}).Debug()
	_, err = util.Execute(RANCHER_MOUNT_BINARY, cmdline)
	if err != nil {
		log.Error("Failed mount, ", err)
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
	_, err := util.Execute(RANCHER_MOUNT_BINARY, cmdline)
	if err != nil {
		log.Error("Failed umount, ", err)
		return err
	}
	return nil
}

func checkRancherMountVersion() error {
	cmdline := []string{"-V"}
	if err := util.CheckBinaryVersion(RANCHER_MOUNT_BINARY, RANCHER_MOUNT_MIN_VERSION, cmdline); err != nil {
		return err
	}
	return nil
}

func CheckEnvironment(driver Driver) error {
	if err := checkRancherMountVersion(); err != nil {
		return err
	}
	if err := driver.CheckEnvironment(); err != nil {
		return err
	}
	return nil
}
