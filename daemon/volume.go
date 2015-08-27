package daemon

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
	"net/http"
	"path/filepath"
	"strconv"

	. "github.com/rancher/convoy/logging"
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

func (v *Volume) ConfigFile() (string, error) {
	if v.UUID == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume UUID")
	}
	if v.configPath == "" {
		return "", fmt.Errorf("BUG: Invalid empty volume config path")
	}
	return filepath.Join(v.configPath, VOLUME_CFG_PREFIX+v.UUID+CFG_POSTFIX), nil
}

func (s *daemon) loadVolume(uuid string) *Volume {
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

func (s *daemon) saveVolume(volume *Volume) error {
	volume.configPath = s.Root
	return util.ObjectSave(volume)
}

func (s *daemon) deleteVolume(volume *Volume) error {
	volume.configPath = s.Root
	return util.ObjectDelete(volume)
}

func (s *daemon) loadVolumeByName(name string) *Volume {
	uuid := s.NameUUIDIndex.Get(name)
	if uuid == "" {
		return nil
	}
	return s.loadVolume(uuid)
}

func (s *daemon) processVolumeCreate(request *api.VolumeCreateRequest) (*Volume, error) {
	volumeName := request.Name
	driverName := request.DriverName

	existedVolume := s.loadVolumeByName(volumeName)
	if existedVolume != nil {
		return nil, fmt.Errorf("Volume name %v already associate locally with volume %v ", volumeName, existedVolume.UUID)
	}

	volumeUUID := uuid.New()
	if volumeName == "" {
		volumeName = "volume-" + volumeUUID[:8]
		for s.NameUUIDIndex.Get(volumeName) != "" {
			volumeUUID = uuid.New()
			volumeName = "volume-" + volumeUUID[:8]
		}
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
		convoydriver.OPT_SIZE:        strconv.FormatInt(request.Size, 10),
		convoydriver.OPT_BACKUP_URL:  util.UnescapeURL(request.BackupURL),
		convoydriver.OPT_VOLUME_NAME: request.Name,
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:      LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:       LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT:      LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:      volumeUUID,
		LOG_FIELD_VOLUME_NAME: volumeName,
		LOG_FIELD_OPTS:        opts,
	}).Debug()
	if err := volOps.CreateVolume(volumeUUID, opts); err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volumeUUID,
	}).Debug("Created volume")

	volume := &Volume{
		UUID:        volumeUUID,
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

func (s *daemon) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		return err
	}
	if request.Verbose {
		return writeResponseOutput(w, api.VolumeResponse{
			UUID:        volume.UUID,
			Name:        volume.Name,
			Driver:      volume.DriverName,
			CreatedTime: volume.CreatedTime,
			DriverInfo:  driverInfo,
			Snapshots:   map[string]api.SnapshotResponse{},
		})
	}
	return writeStringResponse(w, volume.UUID)
}

func (s *daemon) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	request := &api.VolumeDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	if err := util.CheckUUID(request.VolumeUUID); err != nil {
		return err
	}

	return s.processVolumeDelete(request)
}

func (s *daemon) processVolumeDelete(request *api.VolumeDeleteRequest) error {
	uuid := request.VolumeUUID
	volume := s.loadVolume(uuid)
	if volume == nil {
		return fmt.Errorf("Cannot find volume %s", uuid)
	}

	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return err
	}

	opts := map[string]string{
		convoydriver.OPT_REFERENCE_ONLY: strconv.FormatBool(request.ReferenceOnly),
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: uuid,
	}).Debug()
	if err := volOps.DeleteVolume(uuid, opts); err != nil {
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
	for _, snapshot := range volume.Snapshots {
		if err := s.UUIDIndex.Delete(snapshot.UUID); err != nil {
			return err
		}
		if snapshot.Name != "" {
			if err := s.NameUUIDIndex.Delete(snapshot.Name); err != nil {
				return err
			}
		}
	}
	return s.deleteVolume(volume)
}

func (s *daemon) listVolumeInfo(volume *Volume) (*api.VolumeResponse, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return nil, err
	}

	mountPoint, err := volOps.MountPoint(volume.UUID)
	if err != nil {
		return nil, err
	}
	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		return nil, err
	}
	resp := &api.VolumeResponse{
		UUID:        volume.UUID,
		Name:        volume.Name,
		Driver:      volume.DriverName,
		MountPoint:  mountPoint,
		CreatedTime: volume.CreatedTime,
		DriverInfo:  driverInfo,
		Snapshots:   make(map[string]api.SnapshotResponse),
	}
	for uuid, snapshot := range volume.Snapshots {
		driverInfo, err := s.getSnapshotDriverInfo(uuid, volume)
		if err != nil {
			return nil, err
		}
		resp.Snapshots[uuid] = api.SnapshotResponse{
			UUID:        uuid,
			Name:        snapshot.Name,
			CreatedTime: snapshot.CreatedTime,
			DriverInfo:  driverInfo,
		}
	}
	return resp, nil
}

func (s *daemon) listVolume() ([]byte, error) {
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

func (s *daemon) getVolumeDriverInfo(volume *Volume) (map[string]string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return nil, err
	}
	driverInfo, err := volOps.GetVolumeInfo(volume.UUID)
	if err != nil {
		return nil, err
	}
	driverInfo["Driver"] = volOps.Name()
	return driverInfo, nil
}

func (s *daemon) doVolumeList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	driverSpecific, err := util.GetLowerCaseFlag(r, "driver", false, nil)
	if err != nil {
		return err
	}

	var data []byte
	if driverSpecific == "1" {
		result := make(map[string]map[string]string)
		for _, driver := range s.ConvoyDrivers {
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

func (s *daemon) inspectVolume(volumeUUID string) ([]byte, error) {
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

func (s *daemon) doVolumeInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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

func (s *daemon) doVolumeMount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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

	if request.Verbose {
		return writeResponseOutput(w, api.VolumeResponse{
			UUID:       volumeUUID,
			MountPoint: mountPoint,
		})
	}
	return writeStringResponse(w, mountPoint)
}

func (s *daemon) processVolumeMount(volume *Volume, request *api.VolumeMountRequest) (string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return "", err
	}

	opts := map[string]string{
		convoydriver.OPT_MOUNT_POINT: request.MountPoint,
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

func (s *daemon) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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

func (s *daemon) processVolumeUmount(volume *Volume) error {
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

func (s *daemon) getVolumeMountPoint(volume *Volume) (string, error) {
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

func (s *daemon) doRequestUUID(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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
