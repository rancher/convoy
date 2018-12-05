package daemon

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/objectstore"
	"github.com/rancher/convoy/util"

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"
)

func (s *daemon) doBackupList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.BackupListRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	request.URL = util.UnescapeURL(request.URL)

	opts := map[string]string{
		OPT_VOLUME_NAME: request.VolumeName,
	}
	result := make(map[string]map[string]string)
	for _, driver := range s.ConvoyDrivers {
		backupOps, err := driver.BackupOps()
		if err != nil {
			// Not support backup ops
			continue
		}
		infos, err := backupOps.ListBackup(request.URL, request.Endpoint, opts)
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

func (s *daemon) doBackupInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.BackupListRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	request.URL = util.UnescapeURL(request.URL)
	backupOps, err := s.getBackupOpsForBackup(request.URL, request.Endpoint)
	if err != nil {
		return err
	}

	info, err := backupOps.GetBackupInfo(request.URL, request.Endpoint)
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

func (s *daemon) doBackupCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.BackupCreateRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	request.URL = util.UnescapeURL(request.URL)

	snapshotName := request.SnapshotName
	volumeName := s.SnapshotVolumeIndex.Get(snapshotName)
	if volumeName == "" {
		return fmt.Errorf("Cannot find volume of snapshot %v", snapshotName)
	}

	if !s.snapshotExists(volumeName, snapshotName) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotName, volumeName)
	}

	volume := s.getVolume(volumeName)
	backupOps, err := s.getBackupOpsForVolume(volume)
	if err != nil {
		return err
	}

	volumeInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		return err
	}

	snapshot, err := s.getSnapshotDriverInfo(snapshotName, volume)
	if err != nil {
		return err
	}

	opts := map[string]string{
		OPT_VOLUME_NAME:           volumeName,
		OPT_VOLUME_CREATED_TIME:   volumeInfo[OPT_VOLUME_CREATED_TIME],
		OPT_SNAPSHOT_CREATED_TIME: snapshot[OPT_SNAPSHOT_CREATED_TIME],
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:       LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:        LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:       LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:     snapshotName,
		LOG_FIELD_VOLUME:       volumeName,
		LOG_FIELD_DRIVER:       backupOps.Name(),
		LOG_FIELD_DEST_URL:     request.URL,
		LOG_FIELD_ENDPOINT_URL: request.Endpoint,
	}).Debug()
	backupURL, err := backupOps.CreateBackup(snapshotName, volumeName, request.URL, request.Endpoint, opts)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:       LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:        LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:       LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:     snapshotName,
		LOG_FIELD_VOLUME:       volumeName,
		LOG_FIELD_DRIVER:       backupOps.Name(),
		LOG_FIELD_DEST_URL:     request.URL,
		LOG_FIELD_ENDPOINT_URL: request.Endpoint,
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

func (s *daemon) doBackupDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.BackupDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}
	request.URL = util.UnescapeURL(request.URL)

	backupOps, err := s.getBackupOpsForBackup(request.URL, request.Endpoint)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:       LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:        LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:       LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL:     request.URL,
		LOG_FIELD_ENDPOINT_URL: request.Endpoint,
		LOG_FIELD_DRIVER:       backupOps.Name(),
	}).Debug()
	if err := backupOps.DeleteBackup(request.URL, request.Endpoint); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:       LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:        LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:       LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL:     request.URL,
		LOG_FIELD_ENDPOINT_URL: request.Endpoint,
		LOG_FIELD_DRIVER:       backupOps.Name(),
	}).Debug()
	return nil
}

func (s *daemon) getBackupOpsForBackup(requestURL, endpointURL string) (BackupOperations, error) {
	driverName := ""

	if _, err := objectstore.GetObjectStoreDriver(requestURL, endpointURL); err == nil {
		// Known objectstore driver
		objVolume, err := objectstore.LoadVolume(requestURL, endpointURL)
		if err != nil {
			return nil, err
		}
		driverName = objVolume.Driver
	} else {
		// Try Convoy driver
		u, err := url.Parse(requestURL)
		if err != nil {
			return nil, err
		}
		driverName = u.Scheme
	}
	driver := s.ConvoyDrivers[driverName]
	if driver == nil {
		return nil, fmt.Errorf("Cannot find driver %v for restoring", driverName)
	}
	return driver.BackupOps()
}
