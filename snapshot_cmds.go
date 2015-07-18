package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/util"
	"net/http"
	"net/url"

	. "github.com/rancher/rancher-volume/logging"
)

var (
	snapshotCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a snapshot for certain volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "name or uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  KEY_NAME,
				Usage: "name of snapshot",
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
				Usage: "name or uuid of snapshot",
			},
		},
		Action: cmdSnapshotDelete,
	}

	snapshotInspectCmd = cli.Command{
		Name:  "inspect",
		Usage: "inspect an snapshot",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "name or uuid of snapshot, must be used with volume uuid",
			},
		},
		Action: cmdSnapshotInspect,
	}

	snapshotCmd = cli.Command{
		Name:  "snapshot",
		Usage: "snapshot related operations",
		Subcommands: []cli.Command{
			snapshotCreateCmd,
			snapshotDeleteCmd,
			snapshotInspectCmd,
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
	volumeUUID, err := getOrRequestUUID(c, KEY_VOLUME, true)
	snapshotName, err := getName(c, KEY_NAME, false, err)
	if err != nil {
		return err
	}

	if snapshotName != "" {
		v.Set(KEY_NAME, snapshotName)
	}

	request := "/volumes/" + volumeUUID + "/snapshots/create?" + v.Encode()

	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doSnapshotCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error
	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	snapshotName, err := getName(r, KEY_NAME, false, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	uuid := uuid.New()

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

	snapshot := Snapshot{
		UUID:        uuid,
		VolumeUUID:  volumeUUID,
		Name:        snapshotName,
		CreatedTime: util.Now(),
	}
	//TODO: error handling
	volume.Snapshots[uuid] = snapshot
	if err := s.UUIDIndex.Add(snapshot.UUID); err != nil {
		return err
	}
	if err := s.SnapshotVolumeIndex.Add(snapshot.UUID, volume.UUID); err != nil {
		return err
	}
	if snapshot.Name != "" {
		if err := s.NameUUIDIndex.Add(snapshot.Name, snapshot.UUID); err != nil {
			return err
		}
	}
	if err := s.saveVolume(volume); err != nil {
		return err
	}
	return writeResponseOutput(w, api.SnapshotResponse{
		UUID:        snapshot.UUID,
		VolumeUUID:  snapshot.VolumeUUID,
		Name:        snapshot.Name,
		CreatedTime: snapshot.CreatedTime,
	})
}

func cmdSnapshotDelete(c *cli.Context) {
	if err := doSnapshotDelete(c); err != nil {
		panic(err)
	}
}

func doSnapshotDelete(c *cli.Context) error {
	var err error
	uuid, err := getOrRequestUUID(c, KEY_SNAPSHOT, true)
	if err != nil {
		return err
	}

	request := "/snapshots/" + uuid + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doSnapshotDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotUUID)
	if volumeUUID == "" {
		return fmt.Errorf("cannot find volume for snapshot %v", snapshotUUID)
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

	snapshot := volume.Snapshots[snapshotUUID]
	//TODO: error handling
	if err := s.UUIDIndex.Delete(snapshot.UUID); err != nil {
		return err
	}
	if err := s.SnapshotVolumeIndex.Delete(snapshotUUID); err != nil {
		return err
	}
	if snapshot.Name != "" {
		if err = s.NameUUIDIndex.Delete(snapshot.Name); err != nil {
			return err
		}
	}
	delete(volume.Snapshots, snapshotUUID)
	return s.saveVolume(volume)
}

func cmdSnapshotInspect(c *cli.Context) {
	if err := doSnapshotInspect(c); err != nil {
		panic(err)
	}
}

func doSnapshotInspect(c *cli.Context) error {
	var err error

	uuid, err := getOrRequestUUID(c, KEY_SNAPSHOT, true)
	if err != nil {
		return err
	}

	request := "/snapshots/" + uuid + "/"
	return sendRequestAndPrint("GET", request, nil)
}

func (s *Server) doSnapshotInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	var err error

	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotUUID)
	if volumeUUID == "" {
		return fmt.Errorf("cannot find volume for snapshot %v", snapshotUUID)
	}

	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("cannot find volume %v", volumeUUID)
	}
	if _, exists := volume.Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("cannot find snapshot %v of volume %v", snapshotUUID, volumeUUID)
	}
	resp := api.SnapshotResponse{
		UUID:            snapshotUUID,
		VolumeUUID:      volume.UUID,
		VolumeName:      volume.Name,
		Size:            volume.Size,
		VolumeCreatedAt: volume.CreatedTime,
		Name:            volume.Snapshots[snapshotUUID].Name,
		CreatedTime:     volume.Snapshots[snapshotUUID].CreatedTime,
	}
	data, err := api.ResponseOutput(resp)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
