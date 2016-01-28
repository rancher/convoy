package daemon

import (
	"encoding/json"
	"fmt"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
	"net/http"
	"strconv"

	. "github.com/rancher/convoy/convoydriver"
)

type pluginInfo struct {
	Implements []string
}

type pluginResponse struct {
	Mountpoint string `json:",omitempty"`
	Err        string `json:",omitempty"`
}

type pluginRequest struct {
	Name string
	Opts map[string]string
}

func (s *daemon) dockerActivate(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin activate: %v %v", r.Method, r.RequestURI)
	info := pluginInfo{
		Implements: []string{"VolumeDriver"},
	}
	writeResponseOutput(w, info)
}

func getDockerVolumeName(r *http.Request) (string, error) {
	request, err := getDockerVolumeRequest(r)
	if err != nil {
		return "", err
	}
	return request.Name, nil
}

func getDockerVolumeRequest(r *http.Request) (*pluginRequest, error) {
	request := &pluginRequest{}
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		return nil, err
	}
	log.Debugf("Request from docker: %v", request)
	return request, nil
}

func (s *daemon) getDockerVolume(r *http.Request, create bool) (*Volume, error) {
	request, err := getDockerVolumeRequest(r)
	if err != nil {
		return nil, err
	}

	name := request.Name
	var (
		volume     *Volume
		volumeName string
	)
	if util.ValidateName(name) {
		volumeName = name
		volume = s.getVolumeByName(name)
	} else {
		// Not valid UUID or name
		return nil, fmt.Errorf("Invalid volume %s. Must be only contains 0-9, a-z, dash(-), underscore(_) and dot(.)", name)
	}

	if volume == nil {
		if create {
			log.Debugf("Create a new volume %v for docker", name)

			size, err := util.ParseSize(request.Opts["size"])
			if err != nil {
				return nil, err
			}
			iops := 0
			if request.Opts["iops"] != "" {
				iops, err = strconv.Atoi(request.Opts["iops"])
				if err != nil {
					return nil, err
				}
			}
			prepareForVM := false
			if request.Opts["vm"] != "" {
				prepareForVM, err = strconv.ParseBool(request.Opts["vm"])
				if err != nil {
					return nil, err
				}
			}
			request := &api.VolumeCreateRequest{
				Name:           volumeName,
				DriverName:     request.Opts["driver"],
				Size:           size,
				BackupURL:      request.Opts["backup"],
				DriverVolumeID: request.Opts["id"],
				Type:           request.Opts["type"],
				PrepareForVM:   prepareForVM,
				IOPS:           int64(iops),
			}
			volume, err = s.processVolumeCreate(request)
			if err != nil {
				return nil, err
			}
		}
	}

	return volume, nil
}

func dockerResponse(w http.ResponseWriter, mountPoint string, err error) {
	e := pluginResponse{
		Mountpoint: mountPoint,
	}
	if err != nil {
		e.Err = err.Error()
	}
	writeResponseOutput(w, e)
}

func (s *daemon) dockerCreateVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin create volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, true)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Found volume %v (name %v) for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME])

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerRemoveVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin remove volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		log.Infof("Couldn't find volume. Nothing to remove.")
		dockerResponse(w, "", nil)
		return
	}

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if s.IgnoreDockerDelete {
		log.Debugf("Ignoring remove volume %v (name %v) for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME])
	} else {
		request := &api.VolumeDeleteRequest{
			VolumeUUID: volume.UUID,
			// By default we don't want to remove the volume because probably we're using NFS
			ReferenceOnly: true,
		}
		if err := s.processVolumeDelete(request); err != nil {
			dockerResponse(w, "", err)
			return
		}

		log.Debugf("Removed volume %v (name %v) for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME])
	}

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerMountVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin mount volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		dockerResponse(w, "", fmt.Errorf("Couldn't find volume."))
		return
	}

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Mount volume: %v (name %v) for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME])

	mountPoint, err := s.processVolumeMount(volume, &api.VolumeMountRequest{})
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, mountPoint, nil)
}

func (s *daemon) dockerUnmountVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin unmount volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		log.Infof("Couldn't find volume. Nothing to unmount.")
		dockerResponse(w, "", nil)
		return
	}

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Unmount volume: %v (name %v) for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME])

	if err := s.processVolumeUmount(volume); err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerVolumePath(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	log.Debugf("Handle plugin volume path: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		dockerResponse(w, "", fmt.Errorf("Couldn't find volume."))
		return
	}

	driverInfo, err := s.getVolumeDriverInfo(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	mountPoint, err := s.getVolumeMountPoint(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}
	log.Debugf("Volume: %v (name %v) is mounted at %v for docker", volume.UUID, driverInfo[OPT_VOLUME_NAME], mountPoint)

	dockerResponse(w, mountPoint, nil)
}
