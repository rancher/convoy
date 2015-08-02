package server

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/storagedriver"
	"github.com/rancher/rancher-volume/util"
	"net/http"
	"path/filepath"
	"strconv"

	. "github.com/rancher/rancher-volume/logging"
)

type Volume struct {
	UUID        string
	Name        string
	DriverName  string
	FileSystem  string
	CreatedTime string
	Snapshots   map[string]Snapshot

	configPath string
}

type Snapshot struct {
	UUID        string
	VolumeUUID  string
	Name        string
	CreatedTime string
}

func (v *Volume) ConfigFile(uuid string) (string, error) {
	if v.configPath == "" {
		return "", fmt.Errorf("Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, VOLUME_CFG_PREFIX+uuid+CFG_POSTFIX), nil
}

func (v *Volume) IDField() string {
	return "UUID"
}

func (s *Server) loadVolume(uuid string) *Volume {
	volume := &Volume{
		UUID:       uuid,
		configPath: s.Root,
	}
	if err := util.ObjectLoad(volume); err != nil {
		log.Errorf("Fail to load volume! %v", err)
		return nil
	}
	return volume
}

func (s *Server) saveVolume(volume *Volume) error {
	volume.configPath = s.Root
	return util.ObjectSave(volume)
}

func (s *Server) deleteVolume(volume *Volume) error {
	volume.configPath = s.Root
	return util.ObjectDelete(volume)
}

func (s *Server) loadVolumeByName(name string) *Volume {
	uuid := s.NameUUIDIndex.Get(name)
	if uuid == "" {
		return nil
	}
	return s.loadVolume(uuid)
}

func (s *Server) processVolumeCreate(request *api.VolumeCreateRequest) (*Volume, error) {
	volumeName := request.Name
	driverName := request.DriverName
	backupURL := request.BackupURL

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
		request.Size = objVolume.Size
	}

	if driverName == "" {
		driverName = s.DefaultDriver
	}
	driver, err := s.getDriver(driverName)
	if err != nil {
		return nil, err
	}
	volOps, err := driver.VolumeOps()
	if err != nil {
		return nil, err
	}

	opts := map[string]string{
		storagedriver.OPTS_SIZE: strconv.FormatInt(request.Size, 10),
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      uuid,
		LOG_FIELD_VOLUME_NAME: volumeName,
		LOG_FIELD_OPTS:        opts,
	}).Debug()
	if err := volOps.CreateVolume(uuid, opts); err != nil {
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
		if err := objectstore.RestoreBackup(backupURL, uuid, driver); err != nil {
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
		DriverName:  driverName,
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
	if volume.Name != "" {
		if err := s.NameUUIDIndex.Add(volume.Name, volume.UUID); err != nil {
			return nil, err
		}
	}
	return volume, nil
}

func (s *Server) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.VolumeCreateRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volume, err := s.processVolumeCreate(request)
	if err != nil {
		return err
	}

	return writeResponseOutput(w, api.VolumeResponse{
		UUID:        volume.UUID,
		Driver:      volume.DriverName,
		Name:        volume.Name,
		CreatedTime: volume.CreatedTime,
	})
}

func (s *Server) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.VolumeDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeUUID := request.VolumeUUID
	if err := util.CheckUUID(volumeUUID); err != nil {
		return err
	}

	return s.processVolumeDelete(volumeUUID)
}

func (s *Server) processVolumeDelete(uuid string) error {
	volume := s.loadVolume(uuid)
	if volume == nil {
		return fmt.Errorf("Cannot find volume %s", uuid)
	}

	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug()
	if err := volOps.DeleteVolume(uuid); err != nil {
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
	if volume.Name != "" {
		if err := s.NameUUIDIndex.Delete(volume.Name); err != nil {
			return err
		}
	}
	return s.deleteVolume(volume)
}

func (s *Server) listVolumeInfo(volume *Volume) (*api.VolumeResponse, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return nil, err
	}

	mountPoint, err := volOps.MountPoint(volume.UUID)
	if err != nil {
		return nil, err
	}
	size, err := s.getVolumeSize(volume.UUID)
	if err != nil {
		return nil, err
	}
	resp := &api.VolumeResponse{
		UUID:        volume.UUID,
		Name:        volume.Name,
		Driver:      volume.DriverName,
		Size:        size,
		MountPoint:  mountPoint,
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
	return resp, nil
}

func (s *Server) listVolume() ([]byte, error) {
	resp := make(map[string]api.VolumeResponse)

	volumeUUIDs, err := util.ListConfigIDs(s.Root, VOLUME_CFG_PREFIX, CFG_POSTFIX)
	if err != nil {
		return nil, err
	}

	for _, uuid := range volumeUUIDs {
		volume := s.loadVolume(uuid)
		if volume == nil {
			return nil, fmt.Errorf("Volume list changed for volume %v", uuid)
		}
		r, err := s.listVolumeInfo(volume)
		if err != nil {
			return nil, err
		}
		resp[uuid] = *r
	}

	return api.ResponseOutput(resp)
}

func (s *Server) doVolumeList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	driverSpecific, err := util.GetLowerCaseFlag(r, "driver", false, nil)
	if err != nil {
		return err
	}

	var data []byte
	if driverSpecific == "1" {
		result := make(map[string]map[string]string)
		for _, driver := range s.StorageDrivers {
			volOps, err := driver.VolumeOps()
			if err != nil {
				break
			}
			volumes, err := volOps.ListVolume(map[string]string{})
			if err != nil {
				break
			}
			for k, v := range volumes {
				v["Driver"] = driver.Name()
				result[k] = v
			}
		}
		data, err = api.ResponseOutput(&result)
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
	resp, err := s.listVolumeInfo(volume)
	if err != nil {
		return nil, err
	}
	return api.ResponseOutput(*resp)
}

func (s *Server) doVolumeInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	request := &api.VolumeInspectRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeUUID := request.VolumeUUID
	if err := util.CheckUUID(volumeUUID); err != nil {
		return err
	}

	data, err := s.inspectVolume(volumeUUID)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (s *Server) doVolumeMount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	var err error

	request := &api.VolumeMountRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeUUID := request.VolumeUUID
	if err := util.CheckUUID(volumeUUID); err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	mountPoint, err := s.processVolumeMount(volume, request)
	if err != nil {
		return err
	}

	return writeResponseOutput(w, api.VolumeResponse{
		UUID:       volumeUUID,
		MountPoint: mountPoint,
	})
}

func (s *Server) processVolumeMount(volume *Volume, request *api.VolumeMountRequest) (string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return "", err
	}

	opts := map[string]string{
		storagedriver.OPTS_MOUNT_POINT: request.MountPoint,
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_MOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.UUID,
		LOG_FIELD_OPTS:   opts,
	}).Debug()
	mountPoint, err := volOps.MountVolume(volume.UUID, opts)
	if err != nil {
		return "", err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_LIST,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: mountPoint,
	}).Debug()
	return mountPoint, nil
}

func (s *Server) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.VolumeUmountRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeUUID := request.VolumeUUID
	if err := util.CheckUUID(volumeUUID); err != nil {
		return err
	}
	volume := s.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}

	return s.processVolumeUmount(volume)
}

func (s *Server) processVolumeUmount(volume *Volume) error {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.UUID,
	}).Debug()
	if err := volOps.UmountVolume(volume.UUID); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.UUID,
	}).Debug()

	return nil
}

func (s *Server) getVolumeMountPoint(volume *Volume) (string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return "", err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_MOUNTPOINT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.UUID,
	}).Debug()
	mountPoint, err := volOps.MountPoint(volume.UUID)
	if err != nil {
		return "", err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_MOUNTPOINT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.UUID,
		LOG_FIELD_MOUNTPOINT: mountPoint,
	}).Debug()

	return mountPoint, nil
}

func (s *Server) doRequestUUID(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error
	key, err := util.GetName(r, api.KEY_NAME, true, err)
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

func (s *Server) getVolumeSize(volumeUUID string) (int64, error) {
	volume := s.loadVolume(volumeUUID)
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return 0, err
	}
	infos, err := volOps.GetVolumeInfo(volumeUUID)
	if err != nil {
		return 0, err
	}
	size := infos[storagedriver.OPTS_SIZE]
	if size == "" {
		return 0, nil
	}
	return util.ParseSize(size)
}
