package convoydriver

import (
	"fmt"
	"path/filepath"

	"github.com/Sirupsen/logrus"
)

/*
InitFunc is the initialize function for each ConvoyDriver. Each driver must
implement this function and register itself through Register().

The registered function would be called upon Convoy need a ConvoyDriver
instance, and it would return a valid ConvoyDriver for operation.

The registered function would take a "root" path, used as driver's configuration
file path, and a map of configuration specified for the driver.
*/
type InitFunc func(root string, config map[string]string) (ConvoyDriver, error)

/*
ConvoyDriver interface would provide all the functionality needed for driver
specific handling. Driver can choose to implement some or all of the available
operations interfaces to provide different functionality to Convoy user.
xxxOps() should return error if the functionality is not implemented by the
driver.
*/
type ConvoyDriver interface {
	Name() string
	Info() (map[string]string, error)

	VolumeOps() (VolumeOperations, error)
	SnapshotOps() (SnapshotOperations, error)
	BackupOps() (BackupOperations, error)
}

type Request struct {
	Name    string
	Options map[string]string
}

/*
VolumeOperations is Convoy Driver volume related operations interface. Any
Convoy Driver must implement this interface.
*/
type VolumeOperations interface {
	Name() string
	CreateVolume(req Request) error
	DeleteVolume(req Request) error
	MountVolume(req Request) (string, error)
	UmountVolume(req Request) error
	MountPoint(req Request) (string, error)
	GetVolumeInfo(name string) (map[string]string, error)
	ListVolume(opts map[string]string) (map[string]map[string]string, error)
}

/*
SnapshotOperations is Convoy Driver snapshot related operations interface. Any
Convoy Driver want to operate snapshots must implement this interface.
*/
type SnapshotOperations interface {
	Name() string
	CreateSnapshot(req Request) error
	DeleteSnapshot(req Request) error
	GetSnapshotInfo(req Request) (map[string]string, error)
	ListSnapshot(opts map[string]string) (map[string]map[string]string, error)
}

/*
BackupOperations is Convoy Driver backup related operations interface. Any
Convoy Driver want to provide backup functionality must implement this
interface. Restore would need to be implemented in
VolumeOperations.CreateVolume() with opts[OPT_BACKUP_URL]
*/
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
	OPT_FORMAT                = "Format"
	OPT_VOLUME_NAME           = "VolumeName"
	OPT_VOLUME_DRIVER_ID      = "VolumeDriverID"
	OPT_VOLUME_TYPE           = "VolumeType"
	OPT_VOLUME_IOPS           = "VolumeIOPS"
	OPT_VOLUME_CREATED_TIME   = "VolumeCreatedAt"
	OPT_SNAPSHOT_NAME         = "SnapshotName"
	OPT_SNAPSHOT_CREATED_TIME = "SnapshotCreatedAt"
	OPT_BACKUP_URL            = "BackupURL"
	OPT_REFERENCE_ONLY        = "ReferenceOnly"
	OPT_PREPARE_FOR_VM        = "PrepareForVM"
	OPT_FILESYSTEM            = "Filesystem"
	OPT_READ_WRITE            = "ReadWrite"
	OPT_BIND_MOUNT            = "BindMount"
	OPT_REMOUNT               = "ReMount"
)

var (
	initializers map[string]InitFunc
	log          = logrus.WithFields(logrus.Fields{"pkg": "convoydriver"})
)

func init() {
	initializers = make(map[string]InitFunc)
}

/*
Register would add specified InitFunc of Convoy Driver to the known driver list.
*/
func Register(name string, initFunc InitFunc) error {
	if _, exists := initializers[name]; exists {
		return fmt.Errorf("Driver %s has already been registered", name)
	}
	initializers[name] = initFunc
	return nil
}

/*
GetDriver would be called each time when a Convoy Driver instance is needed.
*/
func GetDriver(name, root string, config map[string]string) (ConvoyDriver, error) {
	if _, exists := initializers[name]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", name)
	}
	drvRoot := filepath.Join(root, name)
	return initializers[name](drvRoot, config)
}
