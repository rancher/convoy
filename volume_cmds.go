package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/util"
	"net/http"
	"net/url"
	"strconv"

	. "github.com/rancherio/volmgr/logging"
)

var (
	volumeCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a new volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
			cli.IntFlag{
				Name:  "size",
				Usage: "size of volume, in bytes",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "base image's uuid",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd = cli.Command{
		Name:  "delete",
		Usage: "delete a volume with all of it's snapshots",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
		},
		Action: cmdVolumeDelete,
	}

	volumeMountCmd = cli.Command{
		Name:  "mount",
		Usage: "mount a volume to an specific path",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "mountpoint",
				Usage: "mountpoint of volume",
			},
			cli.StringFlag{
				Name:  "fs",
				Value: "ext4",
				Usage: "filesystem of volume(supports ext4 only)",
			},
			cli.BoolFlag{
				Name:  "format",
				Usage: "format or not",
			},
			cli.StringFlag{
				Name:  "option",
				Usage: "mount options",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeMount,
	}

	volumeUmountCmd = cli.Command{
		Name:  "umount",
		Usage: "umount a volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeUmount,
	}

	volumeListCmd = cli.Command{
		Name:  "list",
		Usage: "list all managed volumes",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume, if not supplied, would list all volumes",
			},
			cli.StringFlag{
				Name:  KEY_SNAPSHOT,
				Usage: "uuid of snapshot, must be used with volume uuid",
			},
		},
		Action: cmdVolumeList,
	}

	volumeCmd = cli.Command{
		Name:  "volume",
		Usage: "volume related operations",
		Subcommands: []cli.Command{
			volumeCreateCmd,
			volumeDeleteCmd,
			volumeMountCmd,
			volumeUmountCmd,
			volumeListCmd,
		},
	}
)

func getVolumeCfgName(uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("Invalid volume UUID specified: %v", uuid)
	}
	return "volume_" + uuid + ".json", nil
}

func (config *Config) loadVolume(uuid string) *Volume {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return nil
	}
	if !util.ConfigExists(config.Root, cfgName) {
		return nil
	}
	volume := &Volume{}
	if err := util.LoadConfig(config.Root, cfgName, volume); err != nil {
		log.Error("Failed to load volume json ", cfgName)
		return nil
	}
	return volume
}

func (config *Config) saveVolume(volume *Volume) error {
	uuid := volume.UUID
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return util.SaveConfig(config.Root, cfgName, volume)
}

func (config *Config) deleteVolume(uuid string) error {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return util.RemoveConfig(config.Root, cfgName)
}

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
}

func doVolumeCreate(c *cli.Context) error {
	var err error

	v := url.Values{}
	size := int64(c.Int("size"))
	if size == 0 {
		return genRequiredMissingError("size")
	}
	volumeUUID, err := getLowerCaseFlag(c, KEY_VOLUME, false, err)
	imageUUID, err := getLowerCaseFlag(c, KEY_IMAGE, false, err)
	if err != nil {
		return err
	}
	v.Set("size", strconv.FormatInt(size, 10))
	if volumeUUID != "" {
		v.Set(KEY_VOLUME, volumeUUID)
	}
	if imageUUID != "" {
		v.Set(KEY_IMAGE, imageUUID)
	}

	request := "/volumes/create?" + v.Encode()

	return sendRequest("POST", request, nil)
}

func (s *Server) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	size, err := strconv.ParseInt(r.FormValue("size"), 10, 64)
	if size == 0 {
		return genRequiredMissingError("size")
	}
	volumeUUID, err := getLowerCaseFlag(r, KEY_VOLUME, false, err)
	imageUUID, err := getLowerCaseFlag(r, KEY_IMAGE, false, err)
	if err != nil {
		return err
	}

	uuid := uuid.New()
	if volumeUUID != "" {
		if s.loadVolume(volumeUUID) != nil {
			return fmt.Errorf("Duplicate volume UUID detected!")
		}
		uuid = volumeUUID
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
		LOG_FIELD_IMAGE:  imageUUID,
		LOG_FIELD_SIZE:   size,
	}).Debug()
	if err := s.StorageDriver.CreateVolume(uuid, imageUUID, size); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug("Created volume")

	volume := &Volume{
		UUID:       uuid,
		Base:       imageUUID,
		Size:       size,
		MountPoint: "",
		FileSystem: "",
		Snapshots:  make(map[string]bool),
	}
	if err := s.saveVolume(volume); err != nil {
		return err
	}
	return writeResponseOutput(w, api.VolumeResponse{
		UUID: uuid,
		Base: imageUUID,
		Size: size,
	})
}

func cmdVolumeDelete(c *cli.Context) {
	if err := doVolumeDelete(c); err != nil {
		panic(err)
	}
}

func doVolumeDelete(c *cli.Context) error {
	var err error

	uuid, err := getLowerCaseFlag(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	request := "/volumes/" + uuid + "/"

	return sendRequest("DELETE", request, nil)
}

func (s *Server) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	uuid, err := getLowerCaseFlag(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	if s.loadVolume(uuid) == nil {
		return fmt.Errorf("Cannot find volume %s", uuid)
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug()
	if err := s.StorageDriver.DeleteVolume(uuid); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug()
	return s.deleteVolume(uuid)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(c, KEY_VOLUME, false, err)
	snapshotUUID, err := getLowerCaseFlag(c, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}

	if snapshotUUID != "" && volumeUUID == "" {
		return fmt.Errorf("snapshot must be specified with volume")
	}

	request := "/volumes"
	if volumeUUID != "" {
		request += "/" + volumeUUID
	}
	if snapshotUUID != "" {
		request += "/snapshots/" + snapshotUUID
	}

	request += "/"

	return sendRequest("GET", request, nil)
}

func (s *Server) doVolumeList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(objs, KEY_VOLUME, false, err)
	snapshotUUID, err := getLowerCaseFlag(objs, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}

	if snapshotUUID != "" && volumeUUID == "" {
		return fmt.Errorf("snapshot must be specified with volume")
	}
	data, err := s.StorageDriver.ListVolume(volumeUUID, snapshotUUID)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func cmdVolumeMount(c *cli.Context) {
	if err := doVolumeMount(c); err != nil {
		panic(err)
	}
}

func doVolumeMount(c *cli.Context) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(c, KEY_VOLUME, true, err)
	mountPoint, err := getLowerCaseFlag(c, "mountpoint", true, err)
	fs, err := getLowerCaseFlag(c, "fs", true, err)
	if err != nil {
		return err
	}

	option := c.String("option")
	needFormat := c.Bool("format")
	newNS := c.String("switch-ns")

	mountConfig := api.VolumeMountConfig{
		MountPoint: mountPoint,
		FileSystem: fs,
		Options:    option,
		NeedFormat: needFormat,
		NameSpace:  newNS,
	}

	request := "/volumes/" + volumeUUID + "/mount"
	return sendRequest("POST", request, mountConfig)
}

func (s *Server) doVolumeMount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	mountConfig := &api.VolumeMountConfig{}
	err = json.NewDecoder(r.Body).Decode(mountConfig)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_MOUNT,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_MOUNTPOINT:  mountConfig.MountPoint,
		LOG_FIELD_FILESYSTEM:  mountConfig.FileSystem,
		LOG_FIELD_OPTION:      mountConfig.Options,
		LOG_FIELD_NEED_FORMAT: mountConfig.NeedFormat,
		LOG_FIELD_NAMESPACE:   mountConfig.NameSpace,
	}).Debug()
	if err := drivers.Mount(s.StorageDriver, volumeUUID, mountConfig.MountPoint, mountConfig.FileSystem,
		mountConfig.Options, mountConfig.NeedFormat, mountConfig.NameSpace); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_LIST,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_MOUNTPOINT: mountConfig.MountPoint,
	}).Debug()
	volume.MountPoint = mountConfig.MountPoint
	volume.FileSystem = mountConfig.FileSystem
	return s.saveVolume(volume)
}

func cmdVolumeUmount(c *cli.Context) {
	if err := doVolumeUmount(c); err != nil {
		panic(err)
	}
}

func doVolumeUmount(c *cli.Context) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(c, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	newNS := c.String("switch-ns")

	mountConfig := api.VolumeMountConfig{
		NameSpace: newNS,
	}

	request := "/volumes/" + volumeUUID + "/umount"
	return sendRequest("POST", request, mountConfig)
}

func (s *Server) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	volumeUUID, err := getLowerCaseFlag(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	mountConfig := &api.VolumeMountConfig{}
	err = json.NewDecoder(r.Body).Decode(mountConfig)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_MOUNTPOINT: volume.MountPoint,
		LOG_FIELD_NAMESPACE:  mountConfig.NameSpace,
	}).Debug()
	if err := drivers.Unmount(s.StorageDriver, volume.MountPoint, mountConfig.NameSpace); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volumeUUID,
		LOG_FIELD_MOUNTPOINT: volume.MountPoint,
	}).Debug()
	volume.MountPoint = ""
	return s.saveVolume(volume)
}
