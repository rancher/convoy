package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/blockstore"
	"github.com/rancherio/volmgr/util"
	"net/http"
	"net/url"

	. "github.com/rancherio/volmgr/logging"
)

var (
	snapshotBackupCmd = cli.Command{
		Name:  "backup",
		Usage: "backup an snapshot to blockstore",
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
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotBackup,
	}

	snapshotRestoreCmd = cli.Command{
		Name:  "restore",
		Usage: "restore an snapshot from blockstore to volume",
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
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotRestore,
	}

	snapshotRemoveCmd = cli.Command{
		Name:  "remove",
		Usage: "remove an snapshot in blockstore",
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
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdSnapshotRemove,
	}

	blockstoreRegisterCmd = cli.Command{
		Name:  "register",
		Usage: "register a blockstore for current setup, create it if it's not existed yet",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "kind",
				Value: "vfs",
				Usage: "kind of blockstore, only support vfs now",
			},
			cli.StringSliceFlag{
				Name:  "opts",
				Value: &cli.StringSlice{},
				Usage: "options used to register blockstore",
			},
		},
		Action: cmdBlockStoreRegister,
	}

	blockstoreDeregisterCmd = cli.Command{
		Name:  "deregister",
		Usage: "deregister a blockstore from current setup(no data in it would be changed)",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
		},
		Action: cmdBlockStoreDeregister,
	}

	blockstoreAddVolumeCmd = cli.Command{
		Name:  "add-volume",
		Usage: "add a volume to blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
		},
		Action: cmdBlockStoreAddVolume,
	}

	blockstoreRemoveVolumeCmd = cli.Command{
		Name:  "remove-volume",
		Usage: "remove a volume from blockstore. WARNING: ALL THE DATA ABOUT THE VOLUME IN THIS BLOCKSTORE WOULD BE REMOVED!",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
		},
		Action: cmdBlockStoreRemoveVolume,
	}

	blockstoreListCmd = cli.Command{
		Name:  "list-volume",
		Usage: "list volume and snapshots in blockstore",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
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
		Action: cmdBlockStoreListVolume,
	}

	blockstoreAddImageCmd = cli.Command{
		Name:  "add-image",
		Usage: "upload a raw image to blockstore, which can be used as base image later",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
			cli.StringFlag{
				Name:  "image-name",
				Usage: "user defined name of image",
			},
			cli.StringFlag{
				Name:  "image-file",
				Usage: "file name of image, image must already existed in <images-dir>",
			},
		},
		Action: cmdBlockStoreAddImage,
	}

	blockstoreRemoveImageCmd = cli.Command{
		Name:  "remove-image",
		Usage: "remove an image from blockstore, WARNING: ALL THE VOLUMES/SNAPSHOTS BASED ON THAT IMAGE WON'T BE USABLE AFTER",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image, if unspecified, a random one would be generated",
			},
		},
		Action: cmdBlockStoreRemoveImage,
	}

	blockstoreActivateImageCmd = cli.Command{
		Name:  "activate-image",
		Usage: "download a image from blockstore, prepared it to be used as base image",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
		},
		Action: cmdBlockStoreActivateImage,
	}

	blockstoreDeactivateImageCmd = cli.Command{
		Name:  "deactivate-image",
		Usage: "remove local image copy, must be done after all the volumes depends on it removed",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_BLOCKSTORE,
				Usage: "uuid of blockstore",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "uuid of image",
			},
		},
		Action: cmdBlockStoreDeactivateImage,
	}

	blockstoreCmd = cli.Command{
		Name:  "blockstore",
		Usage: "blockstore related operations",
		Subcommands: []cli.Command{
			blockstoreRegisterCmd,
			blockstoreDeregisterCmd,
			blockstoreAddVolumeCmd,
			blockstoreRemoveVolumeCmd,
			blockstoreAddImageCmd,
			blockstoreRemoveImageCmd,
			blockstoreActivateImageCmd,
			blockstoreDeactivateImageCmd,
			blockstoreListCmd,
		},
	}
)

const (
	BLOCKSTORE_PATH = "blockstore"
)

func cmdBlockStoreRegister(c *cli.Context) {
	if err := doBlockStoreRegister(c); err != nil {
		panic(err)
	}
}

func doBlockStoreRegister(c *cli.Context) error {
	kind := c.String("kind")
	if kind == "" {
		return genRequiredMissingError("kind")
	}
	opts := util.SliceToMap(c.StringSlice("opts"))
	if opts == nil {
		return genRequiredMissingError("opts")
	}

	registerConfig := api.BlockStoreRegisterConfig{
		Kind: kind,
		Opts: opts,
	}

	request := "/blockstores/register"
	return sendRequest("POST", request, registerConfig)
}

func (s *Server) doBlockStoreRegister(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	registerConfig := &api.BlockStoreRegisterConfig{}
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
	b, err := blockstore.Register(s.Root, kind, opts)
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

	return writeResponseOutput(w, api.BlockStoreResponse{
		UUID:      b.UUID,
		Kind:      b.Kind,
		BlockSize: b.BlockSize,
	})
}

func cmdBlockStoreDeregister(c *cli.Context) {
	if err := doBlockStoreDeregister(c); err != nil {
		panic(err)
	}
}

func doBlockStoreDeregister(c *cli.Context) error {
	var err error

	uuid, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + uuid + "/"
	return sendRequest("DELETE", request, nil)
}

func (s *Server) doBlockStoreDeregister(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.Deregister(s.Root, blockstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEREGISTER,
		LOG_FIELD_OBJECT:     LOG_OBJECT_BLOCKSTORE,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}

func cmdBlockStoreAddVolume(c *cli.Context) {
	if err := doBlockStoreAddVolume(c); err != nil {
		panic(err)
	}
}

func doBlockStoreAddVolume(c *cli.Context) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/volumes/" + volumeUUID + "/add"
	return sendRequest("POST", request, nil)
}

func (s *Server) doBlockStoreAddVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.AddVolume(s.Root, blockstoreUUID, volumeUUID, volume.Base, volume.Size); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}

func cmdBlockStoreRemoveVolume(c *cli.Context) {
	if err := doBlockStoreRemoveVolume(c); err != nil {
		panic(err)
	}
}

func doBlockStoreRemoveVolume(c *cli.Context) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/volumes/" + volumeUUID + "/"
	return sendRequest("DELETE", request, nil)
}

func (s *Server) doBlockStoreRemoveVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.RemoveVolume(s.Root, blockstoreUUID, volumeUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}

func cmdBlockStoreListVolume(c *cli.Context) {
	if err := doBlockStoreListVolume(c); err != nil {
		panic(err)
	}
}

func doBlockStoreListVolume(c *cli.Context) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(c, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/volumes/" + volumeUUID
	if snapshotUUID != "" {
		request += "/snapshots/" + snapshotUUID
	}
	request += "/"
	return sendRequest("GET", request, nil)
}

func (s *Server) doBlockStoreListVolume(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(objs, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}
	data, err := blockstore.ListVolume(s.Root, blockstoreUUID, volumeUUID, snapshotUUID)
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

	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(c, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/volumes/" + volumeUUID + "/snapshots/" + snapshotUUID + "/backup"
	return sendRequest("POST", request, nil)
}

func (s *Server) doSnapshotBackup(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(objs, KEY_SNAPSHOT, true, err)
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.BackupSnapshot(s.Root, snapshotUUID, volumeUUID, blockstoreUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
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
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	originVolumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	targetVolumeUUID, err := getLowerCaseUUID(c, "target-volume-uuid", true, err)
	snapshotUUID, err := getLowerCaseUUID(c, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	v.Set("target-volume", targetVolumeUUID)
	request := "/blockstores/" + blockstoreUUID + "/volumes/" + originVolumeUUID +
		"/snapshots/" + snapshotUUID + "/restore?" + v.Encode()
	return sendRequest("POST", request, nil)
}

func (s *Server) doSnapshotRestore(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	originVolumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(objs, KEY_SNAPSHOT, true, err)
	targetVolumeUUID, err := getLowerCaseUUID(r, "target-volume", true, err)
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
		LOG_FIELD_BLOCKSTORE:  blockstoreUUID,
	}).Debug()
	if err := blockstore.RestoreSnapshot(s.Root, snapshotUUID, originVolumeUUID,
		targetVolumeUUID, blockstoreUUID, s.StorageDriver); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:       LOG_EVENT_BACKUP,
		LOG_FIELD_OBJECT:      LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:    snapshotUUID,
		LOG_FIELD_ORIN_VOLUME: originVolumeUUID,
		LOG_FIELD_VOLUME:      targetVolumeUUID,
		LOG_FIELD_BLOCKSTORE:  blockstoreUUID,
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
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(c, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(c, KEY_SNAPSHOT, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/volumes/" + volumeUUID + "/snapshots/" + snapshotUUID + "/"
	return sendRequest("DELETE", request, nil)
}

func (s *Server) doSnapshotRemove(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	volumeUUID, err := getLowerCaseUUID(objs, KEY_VOLUME, true, err)
	snapshotUUID, err := getLowerCaseUUID(objs, KEY_SNAPSHOT, true, err)
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.RemoveSnapshot(s.Root, snapshotUUID, volumeUUID, blockstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
		LOG_FIELD_SNAPSHOT:   snapshotUUID,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}

func cmdBlockStoreAddImage(c *cli.Context) {
	if err := doBlockStoreAddImage(c); err != nil {
		panic(err)
	}
}

func doBlockStoreAddImage(c *cli.Context) error {
	var err error
	v := url.Values{}

	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(c, KEY_IMAGE, false, err)
	imageName, err := getLowerCaseFlag(c, "image-name", false, err)
	if err != nil {
		return err
	}
	imageFile := c.String("image-file")
	if imageFile == "" {
		return genRequiredMissingError("image-file")
	}

	imageConfig := api.BlockStoreImageConfig{
		ImageFile: imageFile,
	}
	if imageUUID != "" {
		v.Set(KEY_IMAGE, imageUUID)
	}
	if imageName != "" {
		v.Set("image-name", imageName)
	}

	request := "/blockstores/" + blockstoreUUID + "/images/add?" + v.Encode()
	return sendRequest("POST", request, imageConfig)
}

func (s *Server) doBlockStoreAddImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(r, KEY_IMAGE, false, err)
	imageName, err := getLowerCaseFlag(r, "image-name", false, err)
	if err != nil {
		return err
	}
	imageConfig := &api.BlockStoreImageConfig{}
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	data, err := blockstore.AddImage(s.Root, s.ImagesDir, imageUUID, imageName, imageFile, blockstoreUUID)
	if err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ADD,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	_, err = w.Write(data)
	return err
}

func cmdBlockStoreRemoveImage(c *cli.Context) {
	if err := doBlockStoreRemoveImage(c); err != nil {
		panic(err)
	}
}

func doBlockStoreRemoveImage(c *cli.Context) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/images/" + imageUUID + "/"
	return sendRequest("DELETE", request, nil)
}

func (s *Server) doBlockStoreRemoveImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(objs, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.RemoveImage(s.Root, s.ImagesDir, imageUUID, blockstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_REMOVE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}

func cmdBlockStoreActivateImage(c *cli.Context) {
	if err := doBlockStoreActivateImage(c); err != nil {
		panic(err)
	}
}

func doBlockStoreActivateImage(c *cli.Context) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/images/" + imageUUID + "/activate"
	return sendRequest("POST", request, nil)
}

func (s *Server) doBlockStoreActivateImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(objs, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_IMAGE_DIR:  s.ImagesDir,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.ActivateImage(s.Root, s.ImagesDir, imageUUID, blockstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_ACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()

	imagePath := blockstore.GetImageLocalStorePath(s.ImagesDir, imageUUID)
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

func cmdBlockStoreDeactivateImage(c *cli.Context) {
	if err := doBlockStoreDeactivateImage(c); err != nil {
		panic(err)
	}
}

func doBlockStoreDeactivateImage(c *cli.Context) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(c, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(c, KEY_IMAGE, true, err)
	if err != nil {
		return err
	}

	request := "/blockstores/" + blockstoreUUID + "/images/" + imageUUID + "/deactivate"
	return sendRequest("POST", request, nil)
}

func (s *Server) doBlockStoreDeactivateImage(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	blockstoreUUID, err := getLowerCaseUUID(objs, KEY_BLOCKSTORE, true, err)
	imageUUID, err := getLowerCaseUUID(objs, KEY_IMAGE, true, err)
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
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	if err := blockstore.DeactivateImage(s.Root, s.ImagesDir, imageUUID, blockstoreUUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_DEACTIVATE,
		LOG_FIELD_OBJECT:     LOG_OBJECT_IMAGE,
		LOG_FIELD_IMAGE:      imageUUID,
		LOG_FIELD_BLOCKSTORE: blockstoreUUID,
	}).Debug()
	return nil
}
