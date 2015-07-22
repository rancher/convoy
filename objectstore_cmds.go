package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/util"
	"net/http"

	. "github.com/rancher/rancher-volume/logging"
)

var (
	backupCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a backup in objectstore: create <snapshot>",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_DEST_URL,
				Usage: "required. destination of backup, would be url like s3://bucket@region/path/ or vfs:///path/",
			},
		},
		Action: cmdBackupCreate,
	}

	backupDeleteCmd = cli.Command{
		Name:   "delete",
		Usage:  "delete a backup in objectstore: delete <backup>",
		Action: cmdBackupDelete,
	}

	backupListCmd = cli.Command{
		Name:  "list",
		Usage: "list volume in objectstore: list <dest>",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of volume",
			},
		},
		Action: cmdBackupList,
	}

	backupInspectCmd = cli.Command{
		Name:   "inspect",
		Usage:  "inspect a backup: inspect <backup>",
		Action: cmdBackupInspect,
	}

	backupCmd = cli.Command{
		Name:  "backup",
		Usage: "backup related operations",
		Subcommands: []cli.Command{
			backupCreateCmd,
			backupDeleteCmd,
			backupListCmd,
			backupInspectCmd,
		},
	}
)

const (
	OBJECTSTORE_PATH = "objectstore"
)

func cmdBackupList(c *cli.Context) {
	if err := doBackupList(c); err != nil {
		panic(err)
	}
}

func doBackupList(c *cli.Context) error {
	var err error

	destURL, err := util.GetLowerCaseFlag(c, "", true, err)
	volumeUUID, err := util.GetUUID(c, KEY_VOLUME_UUID, false, err)
	if err != nil {
		return err
	}

	config := &api.BackupListConfig{
		URL:        destURL,
		VolumeUUID: volumeUUID,
	}
	request := "/backups/list"
	return sendRequestAndPrint("GET", request, config)
}

func (s *Server) doBackupList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	config := &api.BackupListConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}
	data, err := objectstore.List(config.VolumeUUID, config.URL)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func cmdBackupInspect(c *cli.Context) {
	if err := doBackupInspect(c); err != nil {
		panic(err)
	}
}

func doBackupInspect(c *cli.Context) error {
	var err error

	backupURL, err := util.GetLowerCaseFlag(c, "", true, err)
	if err != nil {
		return err
	}

	config := &api.BackupListConfig{
		URL: backupURL,
	}
	request := "/backups/inspect"
	return sendRequestAndPrint("GET", request, config)
}

func (s *Server) doBackupInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	config := &api.BackupListConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}
	data, err := objectstore.Inspect(config.URL)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

func cmdBackupCreate(c *cli.Context) {
	if err := doBackupCreate(c); err != nil {
		panic(err)
	}
}

func doBackupCreate(c *cli.Context) error {
	var err error

	destURL, err := util.GetLowerCaseFlag(c, KEY_DEST_URL, true, err)
	if err != nil {
		return err
	}

	snapshotUUID, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	config := &api.BackupCreateConfig{
		URL:          destURL,
		SnapshotUUID: snapshotUUID,
	}

	request := "/backups/create"
	return sendRequestAndPrint("POST", request, config)
}

func (s *Server) doBackupCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	config := &api.BackupCreateConfig{}
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
	objSnapshot := &objectstore.Snapshot{
		UUID:        snapshotUUID,
		VolumeUUID:  volumeUUID,
		Name:        volume.Snapshots[snapshotUUID].Name,
		CreatedTime: volume.Snapshots[snapshotUUID].CreatedTime,
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()
	backupURL, err := objectstore.CreateBackup(objVolume, objSnapshot, config.URL, s.StorageDriver)
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

func cmdBackupDelete(c *cli.Context) {
	if err := doBackupDelete(c); err != nil {
		panic(err)
	}
}

func doBackupDelete(c *cli.Context) error {
	var err error
	backupURL, err := util.GetLowerCaseFlag(c, "", true, err)
	if err != nil {
		return err
	}

	config := &api.BackupDeleteConfig{
		URL: backupURL,
	}
	request := "/backups"
	return sendRequestAndPrint("DELETE", request, config)
}

func (s *Server) doBackupDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	config := &api.BackupDeleteConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_DEST_URL: config.URL,
	}).Debug()
	if err := objectstore.DeleteBackup(config.URL); err != nil {
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
