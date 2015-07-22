package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/util"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	. "github.com/rancher/rancher-volume/logging"
)

var (
	volumeCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a new volume: create [volume_name] [options]",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "size",
				Usage: "size of volume, in bytes, or end in either G or M or K",
			},
			cli.StringFlag{
				Name:  KEY_BACKUP_URL,
				Usage: "create a volume of backup",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd = cli.Command{
		Name:   "delete",
		Usage:  "delete a volume: delete <volume> [options]",
		Action: cmdVolumeDelete,
	}

	volumeMountCmd = cli.Command{
		Name:  "mount",
		Usage: "mount a volume to an specific path: mount <volume> [options]",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "mountpoint",
				Usage: "mountpoint of volume, if not specified, it would be automatic mounted to default mounts-dir",
			},
		},
		Action: cmdVolumeMount,
	}

	volumeUmountCmd = cli.Command{
		Name:   "umount",
		Usage:  "umount a volume: umount <volume> [options]",
		Action: cmdVolumeUmount,
	}

	volumeListCmd = cli.Command{
		Name:  "list",
		Usage: "list all managed volumes",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "driver",
				Usage: "Ask for driver specific info of volumes and snapshots",
			},
		},
		Action: cmdVolumeList,
	}

	volumeInspectCmd = cli.Command{
		Name:   "inspect",
		Usage:  "inspect a certain volume: inspect <volume>",
		Action: cmdVolumeInspect,
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
	uuid := s.NameUUIDIndex.Get(name)
	if uuid == "" {
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
		if err := s.NameUUIDIndex.Add(volume.Name, volume.UUID); err != nil {
			return err
		}
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
		if err := s.NameUUIDIndex.Delete(volume.Name); err != nil {
			return err
		}
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

	name := c.Args().First()
	size, err := getSize(c, err)
	backupURL, err := getLowerCaseFlag(c, KEY_BACKUP_URL, false, err)
	if err != nil {
		return err
	}

	if backupURL != "" && size != 0 {
		return fmt.Errorf("Cannot specify volume size with backup-url. It would be the same size of backup")
	}

	config := &api.VolumeCreateConfig{
		Name:      name,
		Size:      size,
		BackupURL: backupURL,
	}

	request := "/volumes/create"

	return sendRequestAndPrint("POST", request, config)
}

func (s *Server) processVolumeCreate(volumeName string, size int64, backupURL string) (*Volume, error) {
	existedVolume := s.loadVolumeByName(volumeName)
	if existedVolume != nil {
		return nil, fmt.Errorf("Volume name %v already associate locally with volume %v ", volumeName, existedVolume.UUID)
	}

	uuid := uuid.New()

	if backupURL != "" {
		objVolume, err := objectstore.LoadVolume(backupURL)
		if err != nil {
			return nil, err
		}
		size = objVolume.Size
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      uuid,
		LOG_FIELD_VOLUME_NAME: volumeName,
		LOG_FIELD_SIZE:        size,
	}).Debug()
	if err := s.StorageDriver.CreateVolume(uuid, size); err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug("Created volume")

	if backupURL != "" {
		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON:     LOG_REASON_PREPARE,
			LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
			LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
			LOG_FIELD_VOLUME:     uuid,
			LOG_FIELD_BACKUP_URL: backupURL,
		}).Debug()
		//TODO rollback
		if err := objectstore.RestoreBackup(backupURL, uuid, s.StorageDriver); err != nil {
			return nil, err
		}
		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
			LOG_FIELD_EVENT:      LOG_EVENT_BACKUP,
			LOG_FIELD_OBJECT:     LOG_OBJECT_SNAPSHOT,
			LOG_FIELD_VOLUME:     uuid,
			LOG_FIELD_BACKUP_URL: backupURL,
		}).Debug()
	}

	volume := &Volume{
		UUID:        uuid,
		Name:        volumeName,
		Size:        size,
		FileSystem:  "ext4",
		CreatedTime: util.Now(),
		Snapshots:   make(map[string]Snapshot),
	}
	if err := s.saveVolume(volume); err != nil {
		return nil, err
	}
	if err := s.UUIDIndex.Add(volume.UUID); err != nil {
		return nil, err
	}

	return volume, nil
}

func (s *Server) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	config := &api.VolumeCreateConfig{}
	if err := decodeRequest(r, config); err != nil {
		return err
	}

	size := config.Size

	if size == 0 {
		size = s.DefaultVolumeSize
	}

	volume, err := s.processVolumeCreate(config.Name, size, config.BackupURL)
	if err != nil {
		return err
	}

	return writeResponseOutput(w, api.VolumeResponse{
		UUID:        volume.UUID,
		Name:        volume.Name,
		Size:        volume.Size,
		CreatedTime: volume.CreatedTime,
	})
}

func cmdVolumeDelete(c *cli.Context) {
	if err := doVolumeDelete(c); err != nil {
		panic(err)
	}
}

func getOrRequestUUID(c *cli.Context, key string, required bool) (string, error) {
	var err error
	var id string
	if key == "" {
		id = c.Args().First()
	} else {
		id, err = getLowerCaseFlag(c, key, required, err)
		if err != nil {
			return "", err
		}
	}
	if id == "" && !required {
		return "", nil
	}

	if util.ValidateUUID(id) {
		return id, nil
	}

	return requestUUID(id)
}

func requestUUID(id string) (string, error) {
	// Identify by name
	v := url.Values{}
	v.Set(KEY_NAME, id)

	request := "/uuid?" + v.Encode()
	rc, err := sendRequest("GET", request, nil)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	resp := &api.UUIDResponse{}
	if err := json.NewDecoder(rc).Decode(resp); err != nil {
		return "", err
	}
	if resp.UUID == "" {
		return "", fmt.Errorf("Cannot find volume with name or id %v", id)
	}
	return resp.UUID, nil
}

func doVolumeDelete(c *cli.Context) error {
	var err error

	uuid, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	request := "/volumes/" + uuid + "/"

	return sendRequestAndPrint("DELETE", request, nil)
}

func (s *Server) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	uuid, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}

	return s.processVolumeDelete(uuid)
}

func (s *Server) processVolumeDelete(uuid string) error {
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
	if err := s.UUIDIndex.Delete(volume.UUID); err != nil {
		return err
	}
	return s.deleteVolume(volume)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	v := url.Values{}
	if c.Bool("driver") {
		v.Set("driver", "1")
	}

	request := "/volumes/" + v.Encode()
	return sendRequestAndPrint("GET", request, nil)
}

func cmdVolumeInspect(c *cli.Context) {
	if err := doVolumeInspect(c); err != nil {
		panic(err)
	}
}

func doVolumeInspect(c *cli.Context) error {
	var err error

	volumeUUID, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	request := "/volumes/" + volumeUUID + "/"
	return sendRequestAndPrint("GET", request, nil)
}

func getVolumeInfo(volume *Volume) *api.VolumeResponse {
	resp := &api.VolumeResponse{
		UUID:        volume.UUID,
		Name:        volume.Name,
		Size:        volume.Size,
		MountPoint:  volume.MountPoint,
		CreatedTime: volume.CreatedTime,
		Snapshots:   make(map[string]api.SnapshotResponse),
	}
	for uuid, snapshot := range volume.Snapshots {
		resp.Snapshots[uuid] = api.SnapshotResponse{
			UUID:        uuid,
			Name:        snapshot.Name,
			CreatedTime: snapshot.CreatedTime,
		}
	}
	return resp
}

func (s *Server) listVolume() ([]byte, error) {
	resp := api.VolumesResponse{
		Volumes: make(map[string]api.VolumeResponse),
	}

	volumeUUIDs, err := util.ListConfigIDs(s.Root, VOLUME_CFG_PREFIX, CFG_POSTFIX)
	if err != nil {
		return nil, err
	}

	for _, uuid := range volumeUUIDs {
		volume := s.loadVolume(uuid)
		if volume == nil {
			return nil, fmt.Errorf("Volume list changed for volume %v", uuid)
		}
		resp.Volumes[uuid] = *getVolumeInfo(volume)
	}

	return api.ResponseOutput(resp)
}

func (s *Server) doVolumeList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	var err error
	driverSpecific, err := getLowerCaseFlag(r, "driver", false, err)
	if err != nil {
		return err
	}

	var data []byte
	if driverSpecific == "1" {
		data, err = s.StorageDriver.ListVolume("")
	} else {
		data, err = s.listVolume()
	}
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (s *Server) inspectVolume(volumeUUID string) ([]byte, error) {
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return nil, fmt.Errorf("Cannot find volume %v", volumeUUID)
	}
	resp := *getVolumeInfo(volume)
	return api.ResponseOutput(resp)
}

func (s *Server) doVolumeInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	var err error

	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}

	data, err := s.inspectVolume(volumeUUID)
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

	volumeUUID, err := getOrRequestUUID(c, "", true)
	mountPoint, err := getLowerCaseFlag(c, "mountpoint", false, err)
	if err != nil {
		return err
	}

	mountConfig := api.VolumeMountConfig{
		MountPoint: mountPoint,
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
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	if volume.MountPoint != "" {
		return fmt.Errorf("volume %v already mounted at %v as record shows", volumeUUID, volume.MountPoint)
	}

	config := &api.VolumeMountConfig{}
	if err = decodeRequest(r, config); err != nil {
		return err
	}

	config.MountPoint, err = s.getVolumeMountPoint(volumeUUID, config.MountPoint)
	if err != nil {
		return err
	}

	if err = s.processVolumeMount(volume, config); err != nil {
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
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_MOUNT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: mountConfig.MountPoint,
	}).Debug()
	if err := s.StorageDriver.Mount(volume.UUID, mountConfig.MountPoint); err != nil {
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

	volumeUUID, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	request := "/volumes/" + volumeUUID + "/umount"
	return sendRequestAndPrint("POST", request, nil)
}

func (s *Server) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	volumeUUID, err := getUUID(objs, KEY_VOLUME_UUID, true, err)
	if err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	if volume.MountPoint == "" {
		return fmt.Errorf("volume %v hasn't been mounted as record shows", volumeUUID)
	}

	return s.processVolumeUmount(volume)
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

func (s *Server) processVolumeUmount(volume *Volume) error {
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:      LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: volume.MountPoint,
	}).Debug()
	if err := s.StorageDriver.Umount(volume.UUID, volume.MountPoint); err != nil {
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

func (s *Server) doRequestUUID(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	key, err := getLowerCaseFlag(r, KEY_NAME, true, err)
	if err != nil {
		return err
	}

	var uuid string
	resp := &api.UUIDResponse{}

	if util.ValidateName(key) {
		// It's probably a name
		uuid = s.NameUUIDIndex.Get(key)
	}

	if uuid == "" {
		// No luck with name, let's try uuid index
		uuid, _ = s.UUIDIndex.Get(key)
	}

	if uuid != "" {
		resp.UUID = uuid
	}
	return writeResponseOutput(w, resp)
}
