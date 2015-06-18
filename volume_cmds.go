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
			cli.StringFlag{
				Name:  "size",
				Usage: "size of volume, in bytes, or end in either G or M or K",
			},
			cli.StringFlag{
				Name:  KEY_IMAGE,
				Usage: "base image's uuid",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume, if defined, must be locally unique. Must contains only lower case alphabets/numbers/period/underscore",
			},
			cli.BoolFlag{
				Name:  "format",
				Usage: "format or not, only support ext4 now, would be ignored if base image is provided",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd = cli.Command{
		Name:  "delete",
		Usage: "delete a volume with ALL of it's snapshots LOCALLY. Objects in object store would remain intact",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  KEY_VOLUME,
				Usage: "uuid of volume, not necessary if volume-name presents",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume, not necessary if volume-uuid presents",
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
				Usage: "uuid of volume, not necessary if volume-name presents",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume, not necessary if volume-uuid presents",
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
				Usage: "format or not, only support ext4 now",
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
				Usage: "uuid of volume, not necessary if volume-name presents",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume, not necessary if volume-uuid presents",
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
				Usage: "uuid of volume, cannot specified with volume-name. If both not present, would list all volumes",
			},
			cli.StringFlag{
				Name:  KEY_VOLUME_NAME,
				Usage: "name of volume, cannot specified with volume-name. If both not present, would list all volumes",
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

func (s *Server) loadVolumeByName(name string) *Volume {
	uuid, exists := s.NameVolumeMap[name]
	if !exists {
		return nil
	}
	return s.loadVolume(uuid)
}

func (s *Server) saveVolume(volume *Volume) error {
	uuid := volume.UUID
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	if err := util.SaveConfig(s.Root, cfgName, volume); err != nil {
		return err
	}
	if volume.Name != "" {
		if oldUUID, exists := s.NameVolumeMap[volume.Name]; exists {
			if oldUUID != volume.UUID {
				log.Fatalf("BUG: Volume name %v already assign to %v, but %v want it too. How did it pass the test?", volume.Name, oldUUID, uuid)
				return fmt.Errorf("Volume name %v already assign to %v, but %v want it too", volume.Name, oldUUID, uuid)

			}
			return nil
		}
		s.NameVolumeMap[volume.Name] = volume.UUID
	}
	return nil
}

func (s *Server) deleteVolume(volume *Volume) error {
	cfgName, err := getVolumeCfgName(volume.UUID)
	if err != nil {
		return err
	}
	if err := util.RemoveConfig(s.Root, cfgName); err != nil {
		return err
	}
	if volume.Name != "" {
		if _, exists := s.NameVolumeMap[volume.Name]; !exists {
			log.Fatalf("BUG: Volume name %v assign to %v, but doesn't exist in cache", volume.Name, volume.UUID)
			return fmt.Errorf("BUG: Volume name %v assign to %v, but doesn't exist in cache", volume.Name, volume.UUID)

		}
		delete(s.NameVolumeMap, volume.Name)
	}
	return nil
}

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
}

func getSize(c *cli.Context, err error) (int64, error) {
	size, err := getLowerCaseFlag(c, "size", false, err)
	if err != nil {
		return 0, err
	}
	if size == "" {
		return 0, nil
	}
	return util.ParseSize(size)
}

func doVolumeCreate(c *cli.Context) error {
	var err error

	v := url.Values{}
	volumeUUID, err := getUUID(c, KEY_VOLUME, false, err)
	imageUUID, err := getUUID(c, KEY_IMAGE, false, err)
	name, err := getName(c, KEY_VOLUME_NAME, false, err)
	size, err := getSize(c, err)
	if err != nil {
		return err
	}

	needFormat := c.Bool("format")

	v.Set("size", strconv.FormatInt(size, 10))
	if volumeUUID != "" {
		v.Set(KEY_VOLUME, volumeUUID)
	}
	if imageUUID != "" {
		v.Set(KEY_IMAGE, imageUUID)
	}
	if name != "" {
		v.Set(KEY_VOLUME_NAME, name)
	}
	if needFormat && imageUUID == "" {
		v.Set("need-format", "true")
	}

	request := "/volumes/create?" + v.Encode()

	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) processVolumeCreate(volumeUUID, volumeName, imageUUID string, size int64, needFormat bool) (*Volume, error) {
	existedVolume := s.loadVolumeByName(volumeName)
	if existedVolume != nil {
		return nil, fmt.Errorf("Volume name %v already associate locally with volume %v ", volumeName, existedVolume.UUID)
	}

	uuid := uuid.New()
	if volumeUUID != "" {
		if s.loadVolume(volumeUUID) != nil {
			return nil, fmt.Errorf("Duplicate volume UUID detected!")
		}
		uuid = volumeUUID
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      uuid,
		LOG_FIELD_VOLUME_NAME: volumeName,
		LOG_FIELD_IMAGE:       imageUUID,
		LOG_FIELD_SIZE:        size,
	}).Debug()
	if err := s.StorageDriver.CreateVolume(uuid, imageUUID, size); err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug("Created volume")

	if needFormat {
		if err := drivers.Format(s.StorageDriver, uuid, "ext4"); err != nil {
			//TODO: Rollback
			return nil, err
		}
	}

	volume := &Volume{
		UUID:      uuid,
		Name:      volumeName,
		Base:      imageUUID,
		Size:      size,
		Snapshots: make(map[string]bool),
	}
	if err := s.saveVolume(volume); err != nil {
		return nil, err
	}
	return volume, nil
}

func (s *Server) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	size, err := strconv.ParseInt(r.FormValue("size"), 10, 64)
	volumeUUID, err := getUUID(r, KEY_VOLUME, false, err)
	imageUUID, err := getUUID(r, KEY_IMAGE, false, err)
	volumeName, err := getName(r, KEY_VOLUME_NAME, false, err)
	if err != nil {
		return err
	}
	if size == 0 {
		size = s.DefaultVolumeSize
	}
	needFormat := (r.FormValue("need-format") == "true")

	volume, err := s.processVolumeCreate(volumeUUID, volumeName, imageUUID, size, needFormat)
	if err != nil {
		return err
	}

	return writeResponseOutput(w, api.VolumeResponse{
		UUID: volume.UUID,
		Name: volume.Name,
		Base: volume.Base,
		Size: volume.Size,
	})
}

func cmdVolumeDelete(c *cli.Context) {
	if err := doVolumeDelete(c); err != nil {
		panic(err)
	}
}

func requestVolumeUUID(c *cli.Context) (string, error) {
	var err error
	uuid, err := getUUID(c, KEY_VOLUME, false, err)
	name, err := getName(c, KEY_VOLUME_NAME, false, err)

	if err != nil {
		return "", err
	}

	if uuid == "" && name == "" {
		return "", genRequiredMissingError("volume-uuid or volume-name")
	}

	// Identify by UUID only
	if uuid != "" && name == "" {
		return uuid, nil
	}

	// Identify by name
	v := url.Values{}
	v.Set(KEY_VOLUME_NAME, name)

	request := "/volumes/uuid?" + v.Encode()
	rc, err := sendRequest("GET", request, nil)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	volumes := &api.VolumesResponse{}
	if err := json.NewDecoder(rc).Decode(volumes); err != nil {
		return "", err
	}
	if len(volumes.Volumes) == 0 {
		return "", fmt.Errorf("Cannot find volume named %v", name)
	}
	if len(volumes.Volumes) > 1 {
		return "", fmt.Errorf("FATAL: Multiple volume with name %v?!", name)
	}
	volumeUUID := uuid
	// Should be only one element
	for uuid := range volumes.Volumes {
		if volumeUUID != "" && volumeUUID != uuid {
			return "", fmt.Errorf("Unmatched volume UUID(%v) and name(%v) provided, name belonged to %v",
				volumeUUID, name, uuid)
		}
		return uuid, nil
	}
	return "", fmt.Errorf("FATAL: Has volume map but cannot find one volume in it???")
}

func doVolumeListByName(volumeName, snapshotUUID string, config *api.VolumeListConfig) error {
	v := url.Values{}
	v.Set(KEY_VOLUME_NAME, volumeName)

	if snapshotUUID != "" {
		v.Set(KEY_SNAPSHOT, snapshotUUID)
	}

	request := "/volumes/uuid?" + v.Encode()
	return sendRequestAndPrint("GET", request, config)
}

func doVolumeDelete(c *cli.Context) error {
	var err error

	uuid, err := requestVolumeUUID(c)
	if err != nil {
		return err
	}

	request := "/volumes/" + uuid + "/"

	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	uuid, err := getUUID(objs, KEY_VOLUME, true, err)
	if err != nil {
		return err
	}

	volume := s.loadVolume(uuid)
	if volume == nil {
		return fmt.Errorf("Cannot find volume %s", uuid)
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("Cannot delete volume %s, it hasn't been umounted", uuid)
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
	return s.deleteVolume(volume)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	var err error

	volumeUUID, err := getUUID(c, KEY_VOLUME, false, err)
	snapshotUUID, err := getUUID(c, KEY_SNAPSHOT, false, err)
	volumeName, err := getName(c, KEY_VOLUME_NAME, false, err)
	if err != nil {
		return err
	}

	if volumeUUID != "" && volumeName != "" {
		return fmt.Errorf("Specified both volume UUID and name")
	}

	config := &api.VolumeListConfig{
		DriverSpecific: c.Bool("driver"),
	}

	if snapshotUUID != "" && volumeUUID == "" && volumeName == "" {
		return fmt.Errorf("snapshot must be specified with volume")
	}

	if volumeName != "" {
		return doVolumeListByName(volumeName, snapshotUUID, config)
	}

	return doVolumeListByUUID(volumeUUID, snapshotUUID, config)
}

func doVolumeListByUUID(volumeUUID, snapshotUUID string, config *api.VolumeListConfig) error {
	request := "/volumes"
	if volumeUUID != "" {
		request += "/" + volumeUUID
	}
	if snapshotUUID != "" {
		request += "/snapshots/" + snapshotUUID
	}

	request += "/"

	return sendRequestAndPrint("GET", request, config)
}

func getVolumeInfo(volume *Volume, snapshotUUID string) *api.VolumeResponse {
	resp := &api.VolumeResponse{
		UUID:       volume.UUID,
		Name:       volume.Name,
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
	for uuid := range volume.Snapshots {
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

	volumeUUID, err := getUUID(objs, KEY_VOLUME, false, err)
	snapshotUUID, err := getUUID(objs, KEY_SNAPSHOT, false, err)
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

	volumeUUID, err := requestVolumeUUID(c)
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
	return sendRequestAndPrint("POST", request, mountConfig)
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

	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
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

	volumeUUID, err := requestVolumeUUID(c)
	if err != nil {
		return err
	}

	newNS := c.String("switch-ns")

	mountConfig := api.VolumeMountConfig{
		NameSpace: newNS,
	}

	request := "/volumes/" + volumeUUID + "/umount"
	return sendRequestAndPrint("POST", request, mountConfig)
}

func (s *Server) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	volumeUUID, err := getUUID(objs, KEY_VOLUME, true, err)
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

func (s *Server) doVolumeListByName(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	volumeName, err := getName(r, KEY_VOLUME_NAME, true, err)
	snapshotUUID, err := getUUID(r, KEY_SNAPSHOT, false, err)
	if err != nil {
		return err
	}

	volumes := &api.VolumesResponse{
		Volumes: make(map[string]api.VolumeResponse),
	}
	volume := s.loadVolumeByName(volumeName)
	if volume != nil {
		volumes.Volumes[volume.UUID] = *getVolumeInfo(volume, snapshotUUID)
	}
	return writeResponseOutput(w, volumes)
}
