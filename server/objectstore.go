package server

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/storagedriver"
	"net/http"

	. "github.com/rancher/rancher-volume/logging"
)

func (s *Server) doBackupList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	request := &api.BackupListRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	data, err := objectstore.List(request.VolumeUUID, request.URL)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func (s *Server) doBackupInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	request := &api.BackupListRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	data, err := objectstore.Inspect(request.URL)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func (s *Server) doBackupCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.BackupCreateRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	snapshotUUID := request.SnapshotUUID
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotUUID)
	if volumeUUID == "" {
		return fmt.Errorf("Cannot find volume of snapshot %v", snapshotUUID)
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	volume := s.loadVolume(volumeUUID)
	backupOps, err := s.getBackupOpsForVolume(volume)
	if err != nil {
		return err
	}

	opts := map[string]string{
		storagedriver.OPT_VOLUME_NAME:           volume.Name,
		storagedriver.OPT_FILESYSTEM:            volume.FileSystem,
		storagedriver.OPT_VOLUME_CREATED_TIME:   volume.CreatedTime,
		storagedriver.OPT_SNAPSHOT_NAME:         volume.Snapshots[snapshotUUID].Name,
		storagedriver.OPT_SNAPSHOT_CREATED_TIME: volume.Snapshots[snapshotUUID].CreatedTime,
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
		LOG_FIELD_DRIVER:   backupOps.Name(),
		LOG_FIELD_DEST_URL: request.URL,
	}).Debug()
	backupURL, err := backupOps.CreateBackup(snapshotUUID, volumeUUID, request.URL, opts)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
		LOG_FIELD_DRIVER:   backupOps.Name(),
		LOG_FIELD_DEST_URL: request.URL,
	}).Debug()

	backup := &api.BackupURLResponse{
		URL: backupURL,
	}
	return sendResponse(w, backup)
}

func (s *Server) doBackupDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.BackupDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	objVolume, err := objectstore.LoadVolume(request.URL)
	if err != nil {
		return err
	}
	driver := s.StorageDrivers[objVolume.Driver]
	if driver == nil {
		return fmt.Errorf("Cannot find driver %v for restoring", objVolume.Driver)
	}
	backupOps, err := driver.BackupOps()
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL: request.URL,
		LOG_FIELD_DRIVER:   backupOps.Name(),
	}).Debug()
	if err := backupOps.DeleteBackup(request.URL); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL: request.URL,
		LOG_FIELD_DRIVER:   backupOps.Name(),
	}).Debug()
	return nil
}
