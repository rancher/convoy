package daemon

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"
)

type Volume struct {
	Name       string
	DriverName string
}

var notFoundAPIError = APIError{
	statusCode: http.StatusNotFound,
	error:      fmt.Sprintf("Volume not found."),
}

func (s *daemon) getVolume(name string) *Volume {
	driver, err := s.getDriverForVolume(name)
	if err != nil {
		return nil
	}

	return &Volume{
		Name:       name,
		DriverName: driver.Name(),
	}
}

func (s *daemon) volumeExists(name string) (bool, error) {
	for _, driver := range s.ConvoyDrivers {
		volOps, err := driver.VolumeOps()
		if err != nil {
			return false, err
		}

		v, err := volOps.GetVolumeInfo(name)
		if err != nil {
			if util.IsNotExistsError(err) {
				continue
			}
			return false, err
		}

		if v != nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *daemon) generateName() (string, error) {
	name := util.GenerateName("volume")
	for {
		exists, err := s.volumeExists(name)
		if err != nil {
			return "", fmt.Errorf("Error occurred while checking if volume %v exists: %v", name, err)
		}
		if !exists {
			return name, nil
		}
		name = util.GenerateName("volume")
	}
}

func (s *daemon) processVolumeCreate(request *api.VolumeCreateRequest) (*Volume, error) {
	volumeName := request.Name
	driverName := request.DriverName

	var err error
	if volumeName == "" {
		volumeName, err = s.generateName()
		if err != nil {
			return nil, err
		}
	} else {
		exists, err := s.volumeExists(volumeName)
		if err != nil {
			return nil, fmt.Errorf("Error occurred while checking if volume %v exists: %v", volumeName, err)
		}
		if exists {
			return nil, fmt.Errorf("Volume %v already exists ", volumeName)
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

	req := Request{
		Name: volumeName,
		Options: map[string]string{
			OPT_SIZE:             strconv.FormatInt(request.Size, 10),
			OPT_BACKUP_URL:       util.UnescapeURL(request.BackupURL),
			OPT_VOLUME_NAME:      volumeName,
			OPT_VOLUME_DRIVER_ID: request.DriverVolumeID,
			OPT_VOLUME_TYPE:      request.Type,
			OPT_VOLUME_IOPS:      strconv.FormatInt(request.IOPS, 10),
			OPT_PREPARE_FOR_VM:   strconv.FormatBool(request.PrepareForVM),
		},
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volumeName,
		LOG_FIELD_OPTS:   req.Options,
	}).Debug()
	if err := volOps.CreateVolume(req); err != nil {
		return nil, err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_CREATE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volumeName,
	}).Debug("Created volume")

	volume := &Volume{
		Name:       volumeName,
		DriverName: driverName,
	}

	if err := s.NameUUIDIndex.Add(volumeName, "exists"); err != nil {
		return nil, err
	}
	return volume, nil
}

func (s *daemon) doVolumeCreate(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
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
			Name:        volume.Name,
			Driver:      volume.DriverName,
			CreatedTime: driverInfo[OPT_VOLUME_CREATED_TIME],
			DriverInfo:  driverInfo,
			Snapshots:   map[string]api.SnapshotResponse{},
		})
	}
	return writeStringResponse(w, volume.Name)
}

func (s *daemon) doVolumeDelete(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.VolumeDeleteRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	if err := util.CheckName(request.VolumeName); err != nil {
		return err
	}

	return s.processVolumeDelete(request)
}

func (s *daemon) processVolumeDelete(request *api.VolumeDeleteRequest) error {
	name := request.VolumeName

	volume := s.getVolume(name)
	if volume == nil {
		return notFoundAPIError
	}

	// In the case of snapshot is not supported, snapshots would be nil
	snapshots, _ := s.listSnapshotDriverInfos(volume)

	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return err
	}

	req := Request{
		Name: name,
		Options: map[string]string{
			OPT_REFERENCE_ONLY: strconv.FormatBool(request.ReferenceOnly),
		},
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: name,
	}).Debug()
	if err := volOps.DeleteVolume(req); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_DELETE,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: name,
	}).Debug()
	if err := s.NameUUIDIndex.Delete(volume.Name); err != nil {
		return err
	}
	if snapshots != nil {
		for snapshotName := range snapshots {
			if err := s.NameUUIDIndex.Delete(snapshotName); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *daemon) listVolumeInfo(volume *Volume) (*api.VolumeResponse, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return nil, err
	}

	req := Request{
		Name:    volume.Name,
		Options: map[string]string{},
	}
	mountPoint, err := volOps.MountPoint(req)
	if err != nil {
		return nil, err
	}
	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		return nil, err
	}
	resp := &api.VolumeResponse{
		Name:        volume.Name,
		Driver:      volume.DriverName,
		MountPoint:  mountPoint,
		CreatedTime: driverInfo[OPT_VOLUME_CREATED_TIME],
		DriverInfo:  driverInfo,
		Snapshots:   make(map[string]api.SnapshotResponse),
	}
	snapshots, err := s.listSnapshotDriverInfos(volume)
	if err != nil {
		//snapshot doesn't exists
		return resp, nil
	}
	for name, snapshot := range snapshots {
		snapshot["Driver"] = volOps.Name()
		resp.Snapshots[name] = api.SnapshotResponse{
			Name:        name,
			CreatedTime: snapshot[OPT_SNAPSHOT_CREATED_TIME],
			DriverInfo:  snapshot,
		}
	}
	return resp, nil
}

func (s *daemon) listVolume() ([]byte, error) {
	resp := make(map[string]api.VolumeResponse)

	volumes := s.getVolumeList()

	for name := range volumes {
		volume := s.getVolume(name)
		if volume == nil {
			return nil, fmt.Errorf("Volume list changed for volume %v", name)
		}
		r, err := s.listVolumeInfo(volume)
		if err != nil {
			return nil, err
		}
		resp[name] = *r
	}

	return api.ResponseOutput(resp)
}

func (s *daemon) getVolumeDriverInfo(volume *Volume) (map[string]string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return nil, err
	}
	driverInfo, err := volOps.GetVolumeInfo(volume.Name)
	if err != nil {
		return nil, err
	}
	driverInfo["Driver"] = volOps.Name()
	return driverInfo, nil
}

func (s *daemon) doVolumeList(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	driverSpecific, err := util.GetFlag(r, "driver", false, nil)
	if err != nil {
		return err
	}

	var data []byte
	if driverSpecific == "1" {
		result := s.getVolumeList()
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

func (s *daemon) inspectVolume(name string) ([]byte, error) {
	volume := s.getVolume(name)
	if volume == nil {
		return nil, notFoundAPIError
	}
	resp, err := s.listVolumeInfo(volume)
	if err != nil {
		return nil, err
	}
	return api.ResponseOutput(*resp)
}

func (s *daemon) doVolumeInspect(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.VolumeInspectRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	name := request.VolumeName
	if err := util.CheckName(name); err != nil {
		return err
	}

	data, err := s.inspectVolume(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func (s *daemon) doVolumeMount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	var err error

	request := &api.VolumeMountRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeName := request.VolumeName
	if err := util.CheckName(volumeName); err != nil {
		return err
	}
	volume := s.getVolume(volumeName)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeName)
	}

	mountPoint, err := s.processVolumeMount(volume, request)
	if err != nil {
		return err
	}

	if request.Verbose {
		return writeResponseOutput(w, api.VolumeResponse{
			Name:       volumeName,
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

	req := Request{
		Name: volume.Name,
		Options: map[string]string{
			OPT_MOUNT_POINT: request.MountPoint,
		},
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_MOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.Name,
		LOG_FIELD_OPTS:   req.Options,
	}).Debug()
	mountPoint, err := volOps.MountVolume(req)
	if err != nil {
		return "", err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_LIST,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.Name,
		LOG_FIELD_MOUNTPOINT: mountPoint,
	}).Debug()
	return mountPoint, nil
}

func (s *daemon) doVolumeUmount(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	request := &api.VolumeUmountRequest{}
	if err := decodeRequest(r, request); err != nil {
		return err
	}

	volumeName := request.VolumeName
	if err := util.CheckName(volumeName); err != nil {
		return err
	}
	volume := s.getVolume(volumeName)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeName)
	}

	return s.processVolumeUmount(volume)
}

func (s *daemon) processVolumeUmount(volume *Volume) error {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return err
	}

	req := Request{
		Name:    volume.Name,
		Options: map[string]string{},
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.Name,
	}).Debug()
	if err := volOps.UmountVolume(req); err != nil {
		return err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_UMOUNT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.Name,
	}).Debug()

	return nil
}

func (s *daemon) getVolumeMountPoint(volume *Volume) (string, error) {
	volOps, err := s.getVolumeOpsForVolume(volume)
	if err != nil {
		return "", err
	}

	req := Request{
		Name:    volume.Name,
		Options: map[string]string{},
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_MOUNTPOINT,
		LOG_FIELD_OBJECT: LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME: volume.Name,
	}).Debug()
	mountPoint, err := volOps.MountPoint(req)
	if err != nil {
		return "", err
	}
	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON:     LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:      LOG_EVENT_MOUNTPOINT,
		LOG_FIELD_OBJECT:     LOG_OBJECT_VOLUME,
		LOG_FIELD_VOLUME:     volume.Name,
		LOG_FIELD_MOUNTPOINT: mountPoint,
	}).Debug()

	return mountPoint, nil
}

func (s *daemon) getDriverForVolume(id string) (ConvoyDriver, error) {
	for _, driver := range s.ConvoyDrivers {
		volOps, err := driver.VolumeOps()
		if err != nil {
			continue
		}
		if volOps == nil {
			panic(fmt.Errorf("Driver %v incorrectly reports VolumeOperations implemented",
				driver.Name()))
		}
		if vol, _ := volOps.GetVolumeInfo(id); vol == nil {
			continue
		}
		return driver, nil
	}
	return nil, fmt.Errorf("Cannot find driver for volume %v", id)
}

func (s *daemon) getVolumeList() map[string]map[string]string {
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
	return result
}
