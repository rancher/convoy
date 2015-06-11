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
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
			cli.BoolFlag{
				Name:  "driver",
				Usage: "Ask for driver specific info of volumes and snapshots",
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
	return VOLUME_CFG_PREFIX + uuid + CFG_POSTFIX, nil
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

	config := &api.VolumeListConfig{
		DriverSpecific: c.Bool("driver"),
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

	return sendRequest("GET", request, config)
}

func getVolumeInfo(volume *Volume, snapshotUUID string) *api.VolumeResponse {
	resp := &api.VolumeResponse{
		UUID:       volume.UUID,
		Base:       volume.Base,
		Size:       volume.Size,
		MountPoint: volume.MountPoint,
		Snapshots:  make(map[string]api.SnapshotResponse),
	}
	if snapshotUUID != "" {
		if _, exists := volume.Snapshots[snapshotUUID]; exists {
			resp.Snapshots[snapshotUUID] = api.SnapshotResponse{
				UUID:       snapshotUUID,
				VolumeUUID: volume.UUID,
			}
		}
		return resp
	}
	for uuid, _ := range volume.Snapshots {
		resp.Snapshots[uuid] = api.SnapshotResponse{
			UUID:       uuid,
			VolumeUUID: volume.UUID,
		}
	}
	return resp
}

func (s *Server) ListVolume(volumeUUID, snapshotUUID string) ([]byte, error) {
	resp := api.VolumesResponse{
		Volumes: make(map[string]api.VolumeResponse),
	}

	var volumeUUIDs []string

	if volumeUUID != "" {
		volumeUUIDs = append(volumeUUIDs, volumeUUID)
	} else {
		volumeUUIDs = util.ListConfigIDs(s.Root, VOLUME_CFG_PREFIX, CFG_POSTFIX)
	}

	for _, uuid := range volumeUUIDs {
		volume := s.loadVolume(uuid)
		if volume == nil {
			return nil, fmt.Errorf("Volume list changed for volume %v", uuid)
		}
		resp.Volumes[uuid] = *getVolumeInfo(volume, snapshotUUID)
	}

	return api.ResponseOutput(resp)
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

	config := &api.VolumeListConfig{}
	err = json.NewDecoder(r.Body).Decode(config)
	if err != nil {
		return err
	}

	var data []byte
	if !config.DriverSpecific {
		data, err = s.ListVolume(volumeUUID, snapshotUUID)
	} else {
		data, err = s.StorageDriver.ListVolume(volumeUUID, snapshotUUID)
	}
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
	mountPoint, err := getLowerCaseFlag(c, "mountpoint", false, err)
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

func (s *Server) getVolumeMountPoint(volumeUUID, mountPoint string) (string, error) {
	if mountPoint != "" {
		return mountPoint, nil
	}
	dir := filepath.Join(s.MountsDir, volumeUUID)
	if err := util.MkdirIfNotExists(dir); err != nil {
		return "", err
	}
	return dir, nil
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

	mountConfig.MountPoint, err = s.getVolumeMountPoint(volumeUUID, mountConfig.MountPoint)
	if err != nil {
		return err
	}

	if err = s.processVolumeMount(volume, mountConfig); err != nil {
		return err
	}

	return writeResponseOutput(w, api.VolumeResponse{
		UUID:       volumeUUID,
		MountPoint: volume.MountPoint,
	})
}

func (s *Server) processVolumeMount(volume *Volume, mountConfig *api.VolumeMountConfig) error {
	if st, err := os.Stat(mountConfig.MountPoint); os.IsNotExist(err) || !st.IsDir() {
		return fmt.Errorf("Mount point %s doesn't exist", mountConfig.MountPoint)
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_MOUNT,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volume.UUID,
		LOG_FIELD_MOUNTPOINT:  mountConfig.MountPoint,
		LOG_FIELD_FILESYSTEM:  mountConfig.FileSystem,
		LOG_FIELD_OPTION:      mountConfig.Options,
		LOG_FIELD_NEED_FORMAT: mountConfig.NeedFormat,
		LOG_FIELD_NAMESPACE:   mountConfig.NameSpace,
	}).Debug()
	if err := drivers.Mount(s.StorageDriver, volume.UUID, mountConfig.MountPoint, mountConfig.FileSystem,
		mountConfig.Options, mountConfig.NeedFormat, mountConfig.NameSpace); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_LIST,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: mountConfig.MountPoint,
	}).Debug()
	volume.MountPoint = mountConfig.MountPoint
	volume.FileSystem = mountConfig.FileSystem
	if err := s.saveVolume(volume); err != nil {
		return err
	}
	return nil
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

	return s.processVolumeUmount(volume, mountConfig)
}

func (s *Server) putVolumeMountPoint(mountPoint string) string {
	if strings.HasPrefix(mountPoint, s.MountsDir) {
		err := os.Remove(mountPoint)
		if err != nil {
			log.Warnf("Cannot cleanup mount point directory %v\n", mountPoint)
		}
	}
	return ""
}

func (s *Server) processVolumeUmount(volume *Volume, mountConfig *api.VolumeMountConfig) error {
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
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
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: volume.MountPoint,
	}).Debug()

	volume.MountPoint = s.putVolumeMountPoint(volume.MountPoint)
	return s.saveVolume(volume)
}
