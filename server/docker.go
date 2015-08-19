package server

import (
	"encoding/json"
	"fmt"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
	"net/http"
)

type PluginInfo struct {
	Implements []string
}

type PluginResponse struct {
	Mountpoint string `json:",omitempty"`
	Err        string `json:",omitempty"`
}

type PluginRequest struct {
	Name string
}

func (s *Server) dockerActivate(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin activate: %v %v", r.Method, r.RequestURI)
	info := PluginInfo{
		Implements: []string{"VolumeDriver"},
	}
	writeResponseOutput(w, info)
}

func getDockerVolumeName(r *http.Request) (string, error) {
	request := &PluginRequest{}
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		return "", err
	}
	return request.Name, nil
}

func (s *Server) getDockerVolume(r *http.Request, create bool) (*Volume, error) {
	name, err := getDockerVolumeName(r)
	if err != nil {
		return nil, err
	}

	var (
		volume     *Volume
		volumeName string
	)
	if util.ValidateName(name) {
		volumeName = name
		volume = s.loadVolumeByName(name)
	} else {
		// Not valid UUID or name
		return nil, fmt.Errorf("Invalid volume name. Must be only contains 0-9, a-z, dash(-), underscore(_) and dot(.)")
	}

	if volume == nil {
		if create {
			log.Debugf("Create a new volume %v for docker", name)

			request := &api.VolumeCreateRequest{
				Name: volumeName,
			}
			volume, err = s.processVolumeCreate(request)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("Cannot find volume %v", name)
		}
	}

	return volume, nil
}

func dockerResponse(w http.ResponseWriter, mountPoint string, err error) {
	e := PluginResponse{
		Mountpoint: mountPoint,
	}
	if err != nil {
		e.Err = err.Error()
	}
	writeResponseOutput(w, e)
}

func (s *Server) dockerCreateVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin create volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, true)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Found volume %v (name %v) for docker", volume.UUID, volume.Name)

	dockerResponse(w, "", nil)
}

func (s *Server) dockerRemoveVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin remove volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	request := &api.VolumeDeleteRequest{
		VolumeUUID: volume.UUID,
		// By default we don't want to remove the volume because probably we're using NFS
		Cleanup: false,
	}
	if err := s.processVolumeDelete(request); err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Removed volume %v (name %v) for docker", volume.UUID, volume.Name)

	dockerResponse(w, "", nil)
}

func (s *Server) dockerMountVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin mount volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Mount volume: %v (name %v) for docker", volume.UUID, volume.Name)

	mountPoint, err := s.processVolumeMount(volume, &api.VolumeMountRequest{})
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, mountPoint, nil)
}

func (s *Server) dockerUnmountVolume(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.Lock()
	defer s.GlobalLock.Unlock()

	log.Debugf("Handle plugin unmount volume: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Unmount volume: %v (name %v) for docker", volume.UUID, volume.Name)

	if err := s.processVolumeUmount(volume); err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, "", nil)
}

func (s *Server) dockerVolumePath(w http.ResponseWriter, r *http.Request) {
	s.GlobalLock.RLock()
	defer s.GlobalLock.RUnlock()

	log.Debugf("Handle plugin volume path: %v %v", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(r, false)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	mountPoint, err := s.getVolumeMountPoint(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}
	log.Debugf("Volume: %v (name %v) is mounted at %v for docker", volume.UUID, volume.Name, mountPoint)

	dockerResponse(w, mountPoint, nil)
}
