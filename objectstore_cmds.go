package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"net/http"

	. "github.com/rancher/rancher-volume/logging"
)

var (
	snapshotBackupCmd = cli.Command{
		Name:  "backup",
		Usage: "backup an snapshot to objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_DEST_URL,
				Usage: "destination of backup, would be url like s3://bucket@region/path/ or vfs:///path/",
			},
		},
		Action: cmdSnapshotBackup,
	}

	snapshotRestoreCmd = cli.Command{
		Name:  "restore",
		Usage: "restore an snapshot from objectstore to volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "target-volume-uuid",
				Usage: "uuid of target volume",
			},
			cli.StringFlag{
				Name:  KEY_BACKUP_URL,
				Usage: "url of backup",
			},
		},
		Action: cmdSnapshotRestore,
	}

	snapshotRemoveCmd = cli.Command{
		Name:  "remove-backup",
		Usage: "remove an snapshot backup in objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BACKUP_URL,
				Usage: "url of backup",
			},
		},
		Action: cmdSnapshotRemove,
	}

	objectstoreListVolumeCmd = cli.Command{
		Name:  "list-volume",
		Usage: "list volume and snapshots in objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_DEST_URL,
				Usage: "destination of backup, would be url like s3://bucket@region/path/ or vfs:///path/",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  KEY_SNAPSHOT_UUID,
				Usage: "uuid of snapshot",
			},
		},
		Action: cmdObjectStoreListVolume,
	}

	objectstoreCmd = cli.Command{
		Name:  "objectstore",
		Usage: "objectstore related operations",
		Subcommands: []cli.Command{
			objectstoreListVolumeCmd,
		},
	}
)

const (
	OBJECTSTORE_PATH = "objectstore"
)

func cmdObjectStoreListVolume(c *cli.Context) {
	if err := doObjectStoreListVolume(c); err != nil {
		panic(err)
	}
}

func doObjectStoreListVolume(c *cli.Context) error {
	var err error

	destURL, err := getLowerCaseFlag(c, KEY_DEST_URL, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT_UUID, false, err)
	if err != nil {
		return err
	}

	config := &api.ObjectStoreListConfig{
		URL:          destURL,
		VolumeUUID:   volumeUUID,
		SnapshotUUID: snapshotUUID,
	}
	request := "/objectstores/list"
	return sendRequestAndPrint("GET", request, config)
}

func (s *Server) doObjectStoreListVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	config := &api.ObjectStoreListConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}
	data, err := objectstore.ListVolume(config.URL, config.VolumeUUID, config.SnapshotUUID)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func cmdSnapshotBackup(c *cli.Context) {
	if err := doSnapshotBackup(c); err != nil {
		panic(err)
	}
}

func doSnapshotBackup(c *cli.Context) error {
	var err error

	destURL, err := getLowerCaseFlag(c, KEY_DEST_URL, true, err)
	if err != nil {
		return err
	}

	snapshotUUID, err := getOrRequestUUID(c, KEY_SNAPSHOT, true)
	if err != nil {
		return err
	}

	config := &api.ObjectStoreBackupConfig{
		URL:          destURL,
		SnapshotUUID: snapshotUUID,
	}

	request := "/objectstores/backup"
	return sendRequestAndPrint("POST", request, config)
}

func (s *Server) doSnapshotBackup(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	config := &api.ObjectStoreBackupConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}
	snapshotUUID := config.SnapshotUUID
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotUUID)
	if volumeUUID == "" {
		return fmt.Errorf("Cannot find volume of snapshot %v", snapshotUUID)
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	volume := s.loadVolume(volumeUUID)
	objVolume := &objectstore.Volume{
		UUID:        volume.UUID,
		Name:        volume.Name,
		Size:        volume.Size,
		FileSystem:  volume.FileSystem,
		CreatedTime: volume.CreatedTime,
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()
	backupURL, err := objectstore.BackupSnapshot(objVolume, snapshotUUID, config.URL, s.StorageDriver)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()

	backup := &api.BackupURLResponse{
		URL: backupURL,
	}
	return sendResponse(w, backup)
}

func cmdSnapshotRestore(c *cli.Context) {
	if err := doSnapshotRestore(c); err != nil {
		panic(err)
	}
}

func doSnapshotRestore(c *cli.Context) error {
	var err error

	backupURL, err := getLowerCaseFlag(c, KEY_BACKUP_URL, true, err)
	targetVolumeUUID, err := getUUID(c, "target-volume-uuid", true, err)
	if err != nil {
		return err
	}

	config := &api.ObjectStoreRestoreConfig{
		URL:              backupURL,
		TargetVolumeUUID: targetVolumeUUID,
	}

	request := "/objectstores/restore"
	return sendRequestAndPrint("POST", request, config)
}

func (s *Server) doSnapshotRestore(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	config := &api.ObjectStoreRestoreConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}

	targetVol := s.loadVolume(config.TargetVolumeUUID)
	if targetVol == nil {
		return fmt.Errorf("volume %v doesn't exist", config.TargetVolumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_VOLUME:     config.TargetVolumeUUID,
		LOG_FIELD_BACKUP_URL: config.URL,
	}).Debug()
	if err := objectstore.RestoreSnapshot(config.URL, config.TargetVolumeUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_VOLUME:     config.TargetVolumeUUID,
		LOG_FIELD_BACKUP_URL: config.URL,
	}).Debug()
	return nil
}

func cmdSnapshotRemove(c *cli.Context) {
	if err := doSnapshotRemove(c); err != nil {
		panic(err)
	}
}

func doSnapshotRemove(c *cli.Context) error {
	var err error
	backupURL, err := getLowerCaseFlag(c, KEY_BACKUP_URL, true, err)
	if err != nil {
		return err
	}

	config := &api.ObjectStoreDeleteConfig{
		URL: backupURL,
	}
	request := "/objectstores"
	return sendRequestAndPrint("DELETE", request, config)
}

func (s *Server) doSnapshotRemove(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	config := &api.ObjectStoreDeleteConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()
	if err := objectstore.RemoveSnapshot(config.URL); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()
	return nil
}
