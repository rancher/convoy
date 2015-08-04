package storagedriver

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"path/filepath"
)

type InitFunc func(root string, config map[string]string) (StorageDriver, error)

type StorageDriver interface {
	Name() string
	Info() (map[string]string, error)

	VolumeOps() (VolumeOperations, error)
	SnapshotOps() (SnapshotOperations, error)
	BackupOps() (BackupOperations, error)
}

type VolumeOperations interface {
	Name() string
	CreateVolume(id string, opts map[string]string) error
	DeleteVolume(id string) error
	MountVolume(id string, opts map[string]string) (string, error)
	UmountVolume(id string) error
	MountPoint(id string) (string, error)
	GetVolumeInfo(id string) (map[string]string, error)
	ListVolume(opts map[string]string) (map[string]map[string]string, error)
}

type SnapshotOperations interface {
	Name() string
	CreateSnapshot(id, volumeID string) error
	DeleteSnapshot(id, volumeID string) error
	GetSnapshotInfo(id, volumeID string) (map[string]string, error)
	ListSnapshot(opts map[string]string) (map[string]map[string]string, error)
}

type BackupOperations interface {
	Name() string
	CreateBackup(snapshotID, volumeID, destURL string, opts map[string]string) (string, error)
	DeleteBackup(backupURL string) error
	GetBackupInfo(backupURL string) (map[string]string, error)
	ListBackup(destURL string, opts map[string]string) (map[string]map[string]string, error)
}

const (
	OPT_MOUNT_POINT           = "MountPoint"
	OPT_SIZE                  = "Size"
	OPT_VOLUME_UUID           = "VolumeUUID"
	OPT_VOLUME_NAME           = "VolumeName"
	OPT_VOLUME_CREATED_TIME   = "VolumeCreatedAt"
	OPT_SNAPSHOT_NAME         = "SnapshotName"
	OPT_SNAPSHOT_CREATED_TIME = "SnapshotCreatedAt"
	OPT_FILESYSTEM            = "FileSystem"
)

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "storagedriver"})
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

func GetDriver(name, root string, config map[string]string) (StorageDriver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	drvRoot := filepath.Join(root, name)
	return initializers[name](drvRoot, config)
}
