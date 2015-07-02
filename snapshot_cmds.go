package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/rancher-volume/api"
	"net/http"
	"net/url"

	. "github.com/rancherio/rancher-volume/logging"
)

var (
	snapshotCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a snapshot for certain volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume for snapshot, if uuid is unspecified",
			},
		},
		Action: cmdSnapshotCreate,
	}

	snapshotDeleteCmd = cli.Command{
		Name:  "delete",
		Usage: "delete a snapshot of certain volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume for snapshot, if uuid is unspecified",
			},
		},
		Action: cmdSnapshotDelete,
	}

	snapshotCmd = cli.Command{
		Name:  "snapshot",
		Usage: "snapshot related operations",
		Subcommands: []cli.Command{
			snapshotCreateCmd,
			snapshotDeleteCmd,
			snapshotBackupCmd,  // in objectstore_cmds.go
			snapshotRestoreCmd, // in objectstore_cmds.go
			snapshotRemoveCmd,  // in objectstore_cmds.go
		},
	}
)

func (config *Config) snapshotExists(volumeUUID, snapshotUUID string) bool {
	volume := config.loadVolume(volumeUUID)
	if volume == nil {
		return false
	}
	_, exists := volume.Snapshots[snapshotUUID]
	return exists
}

func cmdSnapshotCreate(c *cli.Context) {
	if err := doSnapshotCreate(c); err != nil {
		panic(err)
	}
}

func doSnapshotCreate(c *cli.Context) error {
	var err error

	v := url.Values{}
	volumeUUID, err := requestVolumeUUID(c)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}

	if snapshotUUID != "" {
		v.Set(KEY_SNAPSHOT, snapshotUUID)
	}

	request := "/volumes/" + volumeUUID + "/snapshots/create?" + v.Encode()

	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doSnapshotCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(r, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	uuid := uuid.New()
	if snapshotUUID != "" {
		if s.snapshotExists(volumeUUID, snapshotUUID) {
			return fmt.Errorf("Duplicate snapshot UUID for volume %v detected", volumeUUID)
		}
		uuid = snapshotUUID
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: uuid,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()
	if err := s.StorageDriver.CreateSnapshot(uuid, volumeUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: uuid,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()

	volume.Snapshots[uuid] = true
	if err := s.saveVolume(volume); err != nil {
		return err
	}
	return writeResponseOutput(w, api.SnapshotResponse{
		UUID:       uuid,
		VolumeUUID: volumeUUID,
	})
}

func cmdSnapshotDelete(c *cli.Context) {
	if err := doSnapshotDelete(c); err != nil {
		panic(err)
	}
}

func doSnapshotDelete(c *cli.Context) error {
	var err error
	uuid, err := getUUID(c, KEY_SNAPSHOT, true, err)
	volumeUUID, err := requestVolumeUUID(c)
	if err != nil {
		return err
	}

	request := "/volumes/" + volumeUUID + "/snapshots/" + uuid + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doSnapshotDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	volume := s.loadVolume(volumeUUID)
	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:    LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()
	if err := s.StorageDriver.DeleteSnapshot(snapshotUUID, volumeUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:   LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:    LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT:   LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT: snapshotUUID,
		LOG_FIELD_VOLUME:   volumeUUID,
	}).Debug()

	delete(volume.Snapshots, snapshotUUID)
	return s.saveVolume(volume)
}
