package server

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/objectstore"
	"net/http"
	"strings"

	. "github.com/rancher/convoy/logging"
)

func (s *Server) doBackupList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	request := &api.BackupListRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	opts := map[string]string{
		convoydriver.OPT_VOLUME_UUID: request.VolumeUUID,
	}
	result := make(map[string]map[string]string)
	for _, driver := range s.ConvoyDrivers {
		backupOps, err := driver.BackupOps()
		if err != nil {
			// Not support backup ops
			continue
		}
		infos, err := backupOps.ListBackup(request.URL, opts)
		if err != nil {
			return err
		}
		for k, v := range infos {
			result[k] = v
		}
	}

	data, err := api.ResponseOutput(result)
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
	backupOps, err := s.getBackupOpsForBackup(request.URL)
	if err != nil {
		return err
	}

	info, err := backupOps.GetBackupInfo(request.URL)
	if err != nil {
		return err
	}

	data, err := api.ResponseOutput(info)
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
		convoydriver.OPT_VOLUME_NAME:           volume.Name,
		convoydriver.OPT_FILESYSTEM:            volume.FileSystem,
		convoydriver.OPT_VOLUME_CREATED_TIME:   volume.CreatedTime,
		convoydriver.OPT_SNAPSHOT_NAME:         volume.Snapshots[snapshotUUID].Name,
		convoydriver.OPT_SNAPSHOT_CREATED_TIME: volume.Snapshots[snapshotUUID].CreatedTime,
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
	if request.Verbose {
		return sendResponse(w, backup)
	}
	escapedURL := strings.Replace(backupURL, "&", "\\u0026", 1)
	return writeStringResponse(w, escapedURL)
}

func (s *Server) doBackupDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.BackupDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	backupOps, err := s.getBackupOpsForBackup(request.URL)
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

func (s *Server) getBackupOpsForBackup(requestURL string) (convoydriver.BackupOperations, error) {
	objVolume, err := objectstore.LoadVolume(requestURL)
	if err != nil {
		return nil, err
	}
	driver := s.ConvoyDrivers[objVolume.Driver]
	if driver == nil {
		return nil, fmt.Errorf("Cannot find driver %v for restoring", objVolume.Driver)
	}
	return driver.BackupOps()
}
