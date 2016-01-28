package devmapper

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/metadata"
	"github.com/rancher/convoy/objectstore"
	"github.com/rancher/convoy/util"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/rancher/convoy/logging"
)

func (d *Driver) BackupOps() (convoydriver.BackupOperations, error) {
	return d, nil
}

func (d *Driver) HasSnapshot(id, volumeID string) bool {
	_, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return false
	}
	return true
}

func (d *Driver) CompareSnapshot(id, compareID, volumeID string) (*metadata.Mappings, error) {
	includeSame := false
	if compareID == "" || compareID == id {
		compareID = id
		includeSame = true
	}
	snap1, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return nil, err
	}
	snap2, _, err := d.getSnapshotAndVolume(compareID, volumeID)
	if err != nil {
		return nil, err
	}

	dev := d.MetadataDevice
	out, err := util.Execute(THIN_PROVISION_TOOLS_BINARY, []string{"thin_delta",
		"--snap1", strconv.Itoa(snap1.DevID),
		"--snap2", strconv.Itoa(snap2.DevID),
		dev})
	if err != nil {
		return nil, err
	}
	mapping, err := metadata.DeviceMapperThinDeltaParser([]byte(out), d.ThinpoolBlockSize*SECTOR_SIZE, includeSame)
	if err != nil {
		return nil, err
	}
	return mapping, err
}

func (d *Driver) OpenSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:            LOG_REASON_START,
		LOG_FIELD_EVENT:             LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:            LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_VOLUME:            volumeID,
		LOG_FIELD_SNAPSHOT:          id,
		LOG_FIELD_SIZE:              volume.Size,
		DM_LOG_FIELD_SNAPSHOT_DEVID: snapshot.DevID,
	}).Debug()
	if err = devicemapper.ActivateDevice(d.ThinpoolDevice, id, snapshot.DevID, uint64(volume.Size)); err != nil {
		return err
	}
	snapshot.Activated = true

	return util.ObjectSave(volume)
}

func (d *Driver) CloseSnapshot(id, volumeID string) error {
	snapshot, volume, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_START,
		LOG_FIELD_EVENT:    LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: id,
	}).Debug()
	if err := devicemapper.RemoveDevice(id); err != nil {
		return err
	}
	snapshot.Activated = false

	return util.ObjectSave(volume)
}

func (d *Driver) ReadSnapshot(id, volumeID string, offset int64, data []byte) error {
	_, _, err := d.getSnapshotAndVolume(id, volumeID)
	if err != nil {
		return err
	}

	dev := filepath.Join(DM_DIR, id)
	devFile, err := os.Open(dev)
	if err != nil {
		return err
	}
	defer devFile.Close()

	if _, err = devFile.ReadAt(data, offset); err != nil {
		return err
	}

	return nil
}

func (d *Driver) CreateBackup(snapshotID, volumeID, destURL string, opts map[string]string) (string, error) {
	volume := d.blankVolume(volumeID)
	if err := util.ObjectLoad(volume); err != nil {
		return "", err
	}

	objVolume := &objectstore.Volume{
		Name:        volumeID,
		Driver:      d.Name(),
		Size:        volume.Size,
		CreatedTime: opts[convoydriver.OPT_VOLUME_CREATED_TIME],
	}
	objSnapshot := &objectstore.Snapshot{
		Name:        snapshotID,
		CreatedTime: opts[convoydriver.OPT_SNAPSHOT_CREATED_TIME],
	}
	return objectstore.CreateDeltaBlockBackup(objVolume, objSnapshot, destURL, d)
}

func (d *Driver) DeleteBackup(backupURL string) error {
	objVolume, err := objectstore.LoadVolume(backupURL)
	if err != nil {
		return err
	}
	if objVolume.Driver != d.Name() {
		return fmt.Errorf("BUG: Wrong driver handling DeleteBackup(), driver should be %v but is %v", objVolume.Driver, d.Name())
	}
	return objectstore.DeleteDeltaBlockBackup(backupURL)
}

func (d *Driver) GetBackupInfo(backupURL string) (map[string]string, error) {
	objVolume, err := objectstore.LoadVolume(backupURL)
	if err != nil {
		return nil, err
	}
	if objVolume.Driver != d.Name() {
		return nil, fmt.Errorf("BUG: Wrong driver handling DeleteBackup(), driver should be %v but is %v", objVolume.Driver, d.Name())
	}
	return objectstore.GetBackupInfo(backupURL)
}

func (d *Driver) ListBackup(destURL string, opts map[string]string) (map[string]map[string]string, error) {
	return objectstore.List(opts[convoydriver.OPT_VOLUME_NAME], destURL, d.Name())
}
