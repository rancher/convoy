package main

import (
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/util"
	"net/http"
	"net/url"

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
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdSnapshotBackup,
	}

	snapshotRestoreCmd = cli.Command{
		Name:  "restore",
		Usage: "restore an snapshot from objectstore to volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT_UUID,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of origin volume for snapshot",
			},
			cli.StringFlag{
				Name:  "target-volume-uuid",
				Usage: "uuid of target volume",
			},
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdSnapshotRestore,
	}

	snapshotRemoveCmd = cli.Command{
		Name:  "remove-backup",
		Usage: "remove an snapshot backup in objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_SNAPSHOT_UUID,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdSnapshotRemove,
	}

	objectstoreRegisterCmd = cli.Command{
		Name:  "register",
		Usage: "register a objectstore for current setup, create it if it's not existed yet",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "kind",
				Value: "vfs",
				Usage: "kind of objectstore, only support vfs now",
			},
			cli.StringSliceFlag{
				Name:  "opts",
				Value: &cli.StringSlice{},
				Usage: "options used to register objectstore",
			},
		},
		Action: cmdObjectStoreRegister,
	}

	objectstoreDeregisterCmd = cli.Command{
		Name:  "deregister",
		Usage: "deregister a objectstore from current setup(no data in it would be changed)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdObjectStoreDeregister,
	}

	objectstoreListCmd = cli.Command{
		Name:  "list",
		Usage: "list registered objectstores",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdObjectStoreList,
	}

	objectstoreAddVolumeCmd = cli.Command{
		Name:  "add-volume",
		Usage: "add a volume to objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of volume",
			},
		},
		Action: cmdObjectStoreAddVolume,
	}

	objectstoreRemoveVolumeCmd = cli.Command{
		Name:  "remove-volume",
		Usage: "remove a volume from objectstore. WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS OBJECTSTORE WOULD BE REMOVED!",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_UUID,
				Usage: "uuid of volume",
			},
		},
		Action: cmdObjectStoreRemoveVolume,
	}

	objectstoreListVolumeCmd = cli.Command{
		Name:  "list-volume",
		Usage: "list volume and snapshots in objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_OBJECTSTORE,
				Usage: "uuid of objectstore",
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
			objectstoreRegisterCmd,
			objectstoreDeregisterCmd,
			objectstoreAddVolumeCmd,
			objectstoreRemoveVolumeCmd,
			objectstoreListVolumeCmd,
			objectstoreListCmd,
		},
	}
)

const (
	OBJECTSTORE_PATH = "objectstore"
)

func cmdObjectStoreRegister(c *cli.Context) {
	if err := doObjectStoreRegister(c); err != nil {
		panic(err)
	}
}

func doObjectStoreRegister(c *cli.Context) error {
	kind := c.String("kind")
	if kind == "" {
		return genRequiredMissingError("kind")
	}
	opts := util.SliceToMap(c.StringSlice("opts"))
	if opts == nil {
		return genRequiredMissingError("opts")
	}

	registerConfig := api.ObjectStoreRegisterConfig{
		Kind: kind,
		Opts: opts,
	}

	request := "/objectstores/register"
	return sendRequestAndPrint("POST", request, registerConfig)
}

func (s *Server) doObjectStoreRegister(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	registerConfig := &api.ObjectStoreRegisterConfig{}
	err := json.NewDecoder(r.Body).Decode(registerConfig)
	if err != nil {
		return err
	}

	kind := registerConfig.Kind
	opts := registerConfig.Opts
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_REGISTER,
		LOG_FIELD_OBJECT:      LOG_OBJECT_OBJECTSTORE,
		LOG_FIELD_OBJECTSTORE: "uuid-unknown",
		LOG_FIELD_KIND:        kind,
		LOG_FIELD_OPTION:      opts,
	}).Debug()
	b, err := objectstore.Register(s.Root, kind, opts)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_REGISTER,
		LOG_FIELD_OBJECT:      LOG_OBJECT_OBJECTSTORE,
		LOG_FIELD_OBJECTSTORE: b.UUID,
		LOG_FIELD_BLOCKSIZE:   b.BlockSize,
	}).Debug()

	return writeResponseOutput(w, api.ObjectStoreResponse{
		UUID:      b.UUID,
		Kind:      b.Kind,
		BlockSize: b.BlockSize,
	})
}

func cmdObjectStoreDeregister(c *cli.Context) {
	if err := doObjectStoreDeregister(c); err != nil {
		panic(err)
	}
}

func doObjectStoreDeregister(c *cli.Context) error {
	var err error

	uuid, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + uuid + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doObjectStoreDeregister(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error
	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:      LOG_OBJECT_OBJECTSTORE,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.Deregister(s.Root, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:      LOG_OBJECT_OBJECTSTORE,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreAddVolume(c *cli.Context) {
	if err := doObjectStoreAddVolume(c); err != nil {
		panic(err)
	}
}

func doObjectStoreAddVolume(c *cli.Context) error {
	var err error

	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/add"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) processObjectStoreAddVolume(volumeUUID, objectstoreUUID string) error {
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_SIZE:        volume.Size,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.AddVolume(s.Root, objectstoreUUID, volumeUUID, volume.Name, volume.Size); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func (s *Server) doObjectStoreAddVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}
	return s.processObjectStoreAddVolume(volumeUUID, objectstoreUUID)
}

func cmdObjectStoreRemoveVolume(c *cli.Context) {
	if err := doObjectStoreRemoveVolume(c); err != nil {
		panic(err)
	}
}

func doObjectStoreRemoveVolume(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doObjectStoreRemoveVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}
	if s.loadVolume(volumeUUID) == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RemoveVolume(s.Root, objectstoreUUID, volumeUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreListVolume(c *cli.Context) {
	if err := doObjectStoreListVolume(c); err != nil {
		panic(err)
	}
}

func doObjectStoreListVolume(c *cli.Context) error {
	var err error

	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT_UUID, false, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID
	if snapshotUUID != "" {
		request += "/snapshots/" + snapshotUUID
	}
	request += "/"
	return sendRequestAndPrint("GET", request, nil)
}

func (s *Server) doObjectStoreListVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	var err error

	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, false, err)
	if err != nil {
		return err
	}
	data, err := objectstore.ListVolume(s.Root, objectstoreUUID, volumeUUID, snapshotUUID)
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

	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	if err != nil {
		return err
	}

	snapshotUUID, err := getOrRequestUUID(c, KEY_SNAPSHOT, true)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/snapshots/" + snapshotUUID + "/backup"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doSnapshotBackup(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}
	volumeUUID := s.SnapshotVolumeIndex.Get(snapshotUUID)
	if volumeUUID == "" {
		return fmt.Errorf("Cannot find volume of snapshot %v", snapshotUUID)
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	if !objectstore.VolumeExists(s.Root, volumeUUID, objectstoreUUID) {
		log.WithFields(logrus.Fields{
			LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
			LOG_FIELD_VOLUME:      volumeUUID,
			LOG_FIELD_OBJECTSTORE: objectstoreUUID,
		}).Debug("Cannot find volume in objectstore, add it")
		if err := s.processObjectStoreAddVolume(volumeUUID, objectstoreUUID); err != nil {
			return err
		}
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.BackupSnapshot(s.Root, snapshotUUID, volumeUUID, objectstoreUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdSnapshotRestore(c *cli.Context) {
	if err := doSnapshotRestore(c); err != nil {
		panic(err)
	}
}

func doSnapshotRestore(c *cli.Context) error {
	var err error

	v := url.Values{}
	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	originVolumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	targetVolumeUUID, err := getUUID(c, "target-volume-uuid", true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}

	v.Set("target-volume", targetVolumeUUID)
	request := "/objectstores/" + objectstoreUUID + "/volumes/" + originVolumeUUID +
		"/snapshots/" + snapshotUUID + "/restore?" + v.Encode()
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doSnapshotRestore(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	originVolumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, true, err)
	targetVolumeUUID, err := getUUID(r, "target-volume", true, err)
	if err != nil {
		return err
	}

	targetVol := s.loadVolume(targetVolumeUUID)
	if targetVol == nil {
		return fmt.Errorf("volume %v doesn't exist", targetVolumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_ORIN_VOLUME: originVolumeUUID,
		LOG_FIELD_VOLUME:      targetVolumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RestoreSnapshot(s.Root, snapshotUUID, originVolumeUUID,
		targetVolumeUUID, objectstoreUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_ORIN_VOLUME: originVolumeUUID,
		LOG_FIELD_VOLUME:      targetVolumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
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
	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/snapshots/" + snapshotUUID + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doSnapshotRemove(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error
	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT_UUID, true, err)
	if err != nil {
		return err
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RemoveSnapshot(s.Root, snapshotUUID, volumeUUID, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_OBJECTSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreList(c *cli.Context) {
	if err := doObjectStoreList(c); err != nil {
		panic(err)
	}
}

func doObjectStoreList(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_OBJECTSTORE, false, err)
	if err != nil {
		return err
	}

	request := "/objectstores/"
	if objectstoreUUID != "" {
		request += objectstoreUUID + "/"
	}

	return sendRequestAndPrint("GET", request, nil)
}

func (s *Server) doObjectStoreList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error
	objectstoreUUID, err := getUUID(objs, KEY_OBJECTSTORE, false, err)
	if err != nil {
		return err
	}

	data, err := objectstore.List(s.Root, objectstoreUUID)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}
