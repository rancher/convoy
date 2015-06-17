package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/objectstore"
	"github.com/rancherio/volmgr/util"
	"net/http"
	"net/url"

	. "github.com/rancherio/volmgr/logging"
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
				Name:  KEY_VOLUME,
				Usage: "uuid of volume for snapshot",
			},
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
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
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of origin volume for snapshot",
			},
			cli.StringFlag{
				Name:  "target-volume-uuid",
				Usage: "uuid of target volume",
			},
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdSnapshotRestore,
	}

	snapshotRemoveCmd = cli.Command{
		Name:  "remove",
		Usage: "remove an snapshot in objectstore",
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
				Name:  KEY_BLOCKSTORE,
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
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
		},
		Action: cmdObjectStoreDeregister,
	}

	objectstoreAddVolumeCmd = cli.Command{
		Name:  "add-volume",
		Usage: "add a volume to objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
		},
		Action: cmdObjectStoreAddVolume,
	}

	objectstoreRemoveVolumeCmd = cli.Command{
		Name:  "remove-volume",
		Usage: "remove a volume from objectstore. WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS BLOCKSTORE WOULD BE REMOVED!",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
		},
		Action: cmdObjectStoreRemoveVolume,
	}

	objectstoreListCmd = cli.Command{
		Name:  "list-volume",
		Usage: "list volume and snapshots in objectstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot",
			},
		},
		Action: cmdObjectStoreListVolume,
	}

	objectstoreAddImageCmd = cli.Command{
		Name:  "add-image",
		Usage: "upload a raw image to objectstore, which can be used as base image later",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
			cli.StringFlag{
				Name:  "image-name",
				Usage: "user defined name of image. Must contains only lower case alphabets/numbers/period/underscore",
			},
			cli.StringFlag{
				Name:  "image-file",
				Usage: "file name of image, image must already existed in <images-dir>",
			},
		},
		Action: cmdObjectStoreAddImage,
	}

	objectstoreRemoveImageCmd = cli.Command{
		Name:  "remove-image",
		Usage: "remove an image from objectstore, WARNING: ALL THE VOLUMES/SNAPSHOTS BASED ON THAT IMAGE WON'T BE USABLE AFTER",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image, if unspecified, a random one would be generated",
			},
		},
		Action: cmdObjectStoreRemoveImage,
	}

	objectstoreActivateImageCmd = cli.Command{
		Name:  "activate-image",
		Usage: "download a image from objectstore, prepared it to be used as base image",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
		},
		Action: cmdObjectStoreActivateImage,
	}

	objectstoreDeactivateImageCmd = cli.Command{
		Name:  "deactivate-image",
		Usage: "remove local image copy, must be done after all the volumes depends on it removed",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of objectstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
		},
		Action: cmdObjectStoreDeactivateImage,
	}

	objectstoreCmd = cli.Command{
		Name:  "objectstore",
		Usage: "objectstore related operations",
		Subcommands: []cli.Command{
			objectstoreRegisterCmd,
			objectstoreDeregisterCmd,
			objectstoreAddVolumeCmd,
			objectstoreRemoveVolumeCmd,
			objectstoreAddImageCmd,
			objectstoreRemoveImageCmd,
			objectstoreActivateImageCmd,
			objectstoreDeactivateImageCmd,
			objectstoreListCmd,
		},
	}
)

const (
	BLOCKSTORE_PATH = "objectstore"
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
	registerConfig := &api.ObjectStoreRegisterConfig{}
	err := json.NewDecoder(r.Body).Decode(registerConfig)
	if err != nil {
		return err
	}

	kind := registerConfig.Kind
	opts := registerConfig.Opts
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_REGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: "uuid-unknown",
		LOG_FIELD_KIND:       kind,
		LOG_FIELD_OPTION:     opts,
	}).Debug()
	b, err := objectstore.Register(s.Root, kind, opts)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: b.UUID,
		LOG_FIELD_BLOCKSIZE:  b.BlockSize,
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

	uuid, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + uuid + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doObjectStoreDeregister(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.Deregister(s.Root, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
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

	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/add"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doObjectStoreAddVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_IMAGE:      volume.Base,
		LOG_FIELD_SIZE:       volume.Size,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.AddVolume(s.Root, objectstoreUUID, volumeUUID, volume.Name, volume.Base, volume.Size); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreRemoveVolume(c *cli.Context) {
	if err := doObjectStoreRemoveVolume(c); err != nil {
		panic(err)
	}
}

func doObjectStoreRemoveVolume(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doObjectStoreRemoveVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}
	if s.loadVolume(volumeUUID) == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RemoveVolume(s.Root, objectstoreUUID, volumeUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
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

	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, false, err)
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
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, false, err)
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

	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/snapshots/" + snapshotUUID + "/backup"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doSnapshotBackup(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.BackupSnapshot(s.Root, snapshotUUID, volumeUUID, objectstoreUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
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
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	originVolumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	targetVolumeUUID, err := getUUID(c, "target-volume-uuid", true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, true, err)
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
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	originVolumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, true, err)
	targetVolumeUUID, err := getUUID(r, "target-volume", true, err)
	if err != nil {
		return err
	}

	originVol := s.loadVolume(originVolumeUUID)
	if originVol == nil {
		return fmt.Errorf("volume %v doesn't exist", originVolumeUUID)
	}
	if _, exists := originVol.Snapshots[snapshotUUID]; !exists {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, originVolumeUUID)
	}
	targetVol := s.loadVolume(targetVolumeUUID)
	if targetVol == nil {
		return fmt.Errorf("volume %v doesn't exist", targetVolumeUUID)
	}
	if originVol.Size != targetVol.Size || originVol.Base != targetVol.Base {
		return fmt.Errorf("target volume %v doesn't match original volume %v's size or base",
			targetVolumeUUID, originVolumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_ORIN_VOLUME: originVolumeUUID,
		LOG_FIELD_VOLUME:      targetVolumeUUID,
		LOG_FIELD_BLOCKSTORE:  objectstoreUUID,
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
		LOG_FIELD_BLOCKSTORE:  objectstoreUUID,
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
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/volumes/" + volumeUUID + "/snapshots/" + snapshotUUID + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doSnapshotRemove(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	if !s.snapshotExists(volumeUUID, snapshotUUID) {
		return fmt.Errorf("snapshot %v of volume %v doesn't exist", snapshotUUID, volumeUUID)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RemoveSnapshot(s.Root, snapshotUUID, volumeUUID, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreAddImage(c *cli.Context) {
	if err := doObjectStoreAddImage(c); err != nil {
		panic(err)
	}
}

func doObjectStoreAddImage(c *cli.Context) error {
	var err error
	v := url.Values{}

	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(c, KEY_IMAGE, false, err)
	imageName, err := getName(c, "image-name", false, err)
	if err != nil {
		return err
	}
	imageFile := c.String("image-file")
	if imageFile == "" {
		return genRequiredMissingError("image-file")
	}

	imageConfig := api.ObjectStoreImageConfig{
		ImageFile: imageFile,
	}
	if imageUUID != "" {
		v.Set(KEY_IMAGE, imageUUID)
	}
	if imageName != "" {
		v.Set("image-name", imageName)
	}

	request := "/objectstores/" + objectstoreUUID + "/images/add?" + v.Encode()
	return sendRequestAndPrint("POST", request, imageConfig)
}

func (s *Server) doObjectStoreAddImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(r, KEY_IMAGE, false, err)
	imageName, err := getName(r, "image-name", false, err)
	if err != nil {
		return err
	}
	imageConfig := &api.ObjectStoreImageConfig{}
	err = json.NewDecoder(r.Body).Decode(imageConfig)
	if err != nil {
		return err
	}

	imageFile := imageConfig.ImageFile
	if imageFile == "" {
		return genRequiredMissingError("image-file")
	}

	if imageUUID == "" {
		imageUUID = uuid.New()
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_IMAGE_NAME: imageName,
		LOG_FIELD_IMAGE_FILE: imageFile,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	data, err := objectstore.AddImage(s.Root, s.ImagesDir, imageUUID, imageName, imageFile, objectstoreUUID)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	_, err = w.Write(data)
	return err
}

func cmdObjectStoreRemoveImage(c *cli.Context) {
	if err := doObjectStoreRemoveImage(c); err != nil {
		panic(err)
	}
}

func doObjectStoreRemoveImage(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/images/" + imageUUID + "/"
	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doObjectStoreRemoveImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(objs, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.RemoveImage(s.Root, s.ImagesDir, imageUUID, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreActivateImage(c *cli.Context) {
	if err := doObjectStoreActivateImage(c); err != nil {
		panic(err)
	}
}

func doObjectStoreActivateImage(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/images/" + imageUUID + "/activate"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doObjectStoreActivateImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(objs, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.ActivateImage(s.Root, s.ImagesDir, imageUUID, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()

	imagePath := objectstore.GetImageLocalStorePath(s.ImagesDir, imageUUID)
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_DRIVER:     s.Driver,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_FILE: imagePath,
	}).Debug()
	if err := s.StorageDriver.ActivateImage(imageUUID, imagePath); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_IMAGE,
		LOG_FIELD_DRIVER: s.Driver,
		LOG_FIELD_IMAGE:  imageUUID,
	}).Debug()
	return nil
}

func cmdObjectStoreDeactivateImage(c *cli.Context) {
	if err := doObjectStoreDeactivateImage(c); err != nil {
		panic(err)
	}
}

func doObjectStoreDeactivateImage(c *cli.Context) error {
	var err error
	objectstoreUUID, err := getUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/objectstores/" + objectstoreUUID + "/images/" + imageUUID + "/deactivate"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doObjectStoreDeactivateImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	objectstoreUUID, err := getUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getUUID(objs, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_IMAGE,
		LOG_FIELD_DRIVER: s.Driver,
		LOG_FIELD_IMAGE:  imageUUID,
	}).Debug()
	if err := s.StorageDriver.DeactivateImage(imageUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_IMAGE,
		LOG_FIELD_DRIVER: s.Driver,
		LOG_FIELD_IMAGE:  imageUUID,
	}).Debug()

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	if err := objectstore.DeactivateImage(s.Root, s.ImagesDir, imageUUID, objectstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: objectstoreUUID,
	}).Debug()
	return nil
}
