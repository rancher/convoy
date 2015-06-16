package main

import (
	"encoding/json"
	"fmt"
	"github.com/rancherio/volmgr/api"
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
	log.Debugf("Handle plugin activate: %v %v\n", r.Method, r.RequestURI)
	info := PluginInfo{
		Implements: []string{"VolumeDriver"},
	}
	writeResponseOutput(w, info)
}

func (s *Server) getDockerVolume(w http.ResponseWriter, r *http.Request) (*Volume, error) {
	request := &PluginRequest{}
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		return nil, err
	}

	volume := s.loadVolume(request.Name)
	if volume == nil {
		return nil, fmt.Errorf("Cannot find volume %v", request.Name)
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
	log.Debugf("Handle plugin create volume: %v %v\n", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(w, r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Create volume %v for docker\n", volume.UUID)

	dockerResponse(w, "", nil)
}

func (s *Server) dockerRemoveVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin remove volume: %v %v\n", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(w, r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Remove volume %v for docker, nothing would be done\n", volume.UUID)

	dockerResponse(w, "", nil)
}

func (s *Server) dockerMountVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin mount volume: %v %v\n", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(w, r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	mountConfig := &api.VolumeMountConfig{
		FileSystem: "ext4",
		NeedFormat: false,
	}

	mountConfig.MountPoint, err = s.getVolumeMountPoint(volume.UUID, "")
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Mount volume: %v to %v for docker\n", volume.UUID, mountConfig.MountPoint)

	if err := s.processVolumeMount(volume, mountConfig); err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, mountConfig.MountPoint, nil)
}

func (s *Server) dockerUnmountVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin unmount volume: %v %v\n", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(w, r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Unmount volume: %v at %v for docker\n", volume.UUID, volume.MountPoint)

	mountConfig := &api.VolumeMountConfig{}

	if err := s.processVolumeUmount(volume, mountConfig); err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, "", nil)
}

func (s *Server) dockerVolumePath(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin volume path: %v %v\n", r.Method, r.RequestURI)

	volume, err := s.getDockerVolume(w, r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	log.Debugf("Volume: %v is mounted at %v for docker\n", volume.UUID, volume.MountPoint)

	dockerResponse(w, volume.MountPoint, nil)
}
