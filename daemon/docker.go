package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/rancher/convoy/api"
	. "github.com/rancher/convoy/convoydriver"
	"github.com/rancher/convoy/util"
)

type pluginInfo struct {
	Implements []string
}

type pluginResponse struct {
	Mountpoint string          `json:",omitempty"`
	Err        string          `json:",omitempty"`
	Volumes    []*DockerVolume `json:",omitempty"`
	Volume     *DockerVolume   `json:",omitempty"`
}

type DockerVolume struct {
	Name       string `json:",omitempty"`
	Mountpoint string `json:",omitempty"`
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

func convertToPluginRequest(r *http.Request) (*pluginRequest, error) {
	request := &pluginRequest{}
	if err := json.NewDecoder(r.Body).Decode(request); err != nil {
		return nil, err
	}
	log.Debugf("Request from docker: %v", request)
	return request, nil
}

func (s *daemon) createDockerVolume(request *pluginRequest) (*Volume, error) {
	name := request.Name
	log.Debugf("Processing request to create volume %s for docker", name)

	if !util.ValidateName(name) {
		return nil, fmt.Errorf("Invalid volume name %s. Can only contain 0-9, a-z, dash(-), underscore(_) and dot(.)", name)
	}

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
	createReq := &api.VolumeCreateRequest{
		Name:           name,
		DriverName:     request.Opts["driver"],
		Size:           size,
		BackupURL:      request.Opts["backup"],
		DriverVolumeID: request.Opts["id"],
		Type:           request.Opts["type"],
		PrepareForVM:   prepareForVM,
		IOPS:           int64(iops),
	}
	return s.processVolumeCreate(createReq)
}

func (s *daemon) getDockerVolume(r *http.Request) (*Volume, *pluginRequest, error) {
	request, err := convertToPluginRequest(r)
	request.Opts = make(map[string]string)
	//log.Debugf("Request obj is %v.\n Name:%s\n Opts:%v", request, request.Name, request.Opts)

	//This check parses the name to check if there is a need to pick the size from the name.
	sizeRe := regexp.MustCompile(`^(.+)~([1-9]+[0-9]*[GT])$`)
	if matches := sizeRe.FindAllStringSubmatch(request.Name, 1); len(matches) > 0 {
		log.Debugf("Volume name received (%s) needs to be parsed for size", request.Name)
		vsName := matches[0][1]
		vsSize := matches[0][2]

		//update our values
		log.Debugf("Updating volume name to %s for size %s", vsName, vsSize)
		request.Opts["size"] = vsSize
		request.Name = vsName
	}

	if err != nil {
		return nil, nil, err
	}
	volume := s.getVolume(request.Name)
	return volume, request, nil
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
	log.Debugf("Handle plugin create volume: %v %v", r.Method, r.RequestURI)

	volume, request, err := s.getDockerVolume(r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume != nil {
		log.Debugf("Found existing volume for docker %v", volume.Name)
		dockerResponse(w, "", nil)
		return
	}

	volume, err = s.createDockerVolume(request)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerRemoveVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin remove volume: %v %v", r.Method, r.RequestURI)

	if s.IgnoreDockerDelete {
		req, err := convertToPluginRequest(r)
		var name string
		if err != nil {
			name = "unknown"
		} else {
			name = req.Name
		}
		log.Debugf("Ignoring remove volume %v for docker", name)
	} else {
		volume, _, err := s.getDockerVolume(r)
		if err != nil {
			dockerResponse(w, "", err)
			return
		}

		if volume == nil {
			log.Infof("Couldn't find volume. Nothing to remove.")
			dockerResponse(w, "", nil)
			return
		}

		request := &api.VolumeDeleteRequest{
			VolumeName: volume.Name,
			// By default we don't want to remove the volume because probably we're using NFS
			ReferenceOnly: true,
		}
		if err := s.processVolumeDelete(request); err != nil {
			dockerResponse(w, "", err)
			return
		}

		log.Debugf("Removed volume %v for docker", volume.Name)
	}

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerMountVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin mount volume: %v %v", r.Method, r.RequestURI)

	volume, request, err := s.getDockerVolume(r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume != nil {
		_, err := s.isVolumeAttached(volume.Name)
		if util.IsNotAttachedInBackendError(err) {
			log.Debugf("Volume %s is not attached to a device", volume.Name)
			request := &api.VolumeDeleteRequest{
				VolumeName:    volume.Name,
				ReferenceOnly: true,
			}

			// if a volume is not attached then processVolumeDelete() just updates the local state.
			if err := s.processVolumeDelete(request); err != nil {
				log.Warnf("Problem processing volume deletion: %s (continuing despite this error)", err)
			}
			log.Debugf("Volume %s removed from local state, will attempt recreation", volume.Name)
			volume = nil
		}
	}

	if volume == nil {
		if s.CreateOnDockerMount {
			volume, err = s.createDockerVolume(request)
			if err != nil {
				dockerResponse(w, "", err)
				return
			}
			log.Debugf("Created volume for docker during mount %v", volume.Name)
		} else {
			dockerResponse(w, "", fmt.Errorf("Couldn't find volume."))
			return
		}
	}

	log.Debugf("Mount volume: %v for docker", volume.Name)

	mountPoint, err := s.processVolumeMount(volume, &api.VolumeMountRequest{})
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, mountPoint, nil)
}

func (s *daemon) dockerUnmountVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin unmount volume: %v %v", r.Method, r.RequestURI)

	volume, _, err := s.getDockerVolume(r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		log.Infof("Couldn't find volume. Nothing to unmount.")
		dockerResponse(w, "", nil)
		return
	}

	log.Debugf("Unmount volume: %v for docker", volume.Name)

	if err := s.processVolumeUmount(volume); err != nil {
		dockerResponse(w, "", err)
		return
	}

	dockerResponse(w, "", nil)
}

func (s *daemon) dockerVolumePath(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin volume path: %v %v", r.Method, r.RequestURI)

	volume, _, err := s.getDockerVolume(r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		dockerResponse(w, "", fmt.Errorf("Couldn't find volume."))
		return
	}

	mountPoint, err := s.getVolumeMountPoint(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}
	log.Debugf("Volume: %v is mounted at %v for docker", volume.Name, mountPoint)

	dockerResponse(w, mountPoint, nil)
}

func (s *daemon) dockerGetVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin get volume: %v %v", r.Method, r.RequestURI)

	volume, req, err := s.getDockerVolume(r)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	if volume == nil {
		dockerResponse(w, "", fmt.Errorf("Could not find volume %v.", req.Name))
		return
	}

	mountPoint, err := s.getVolumeMountPoint(volume)
	if err != nil {
		dockerResponse(w, "", err)
		return
	}

	response := pluginResponse{
		Volume: &DockerVolume{
			Name:       volume.Name,
			Mountpoint: mountPoint,
		},
	}

	log.Debugf("Found volume %v for docker", volume.Name)

	writeResponseOutput(w, response)
}

func (s *daemon) dockerListVolume(w http.ResponseWriter, r *http.Request) {
	log.Debugf("Handle plugin list volume: %v %v", r.Method, r.RequestURI)

	vols := []*DockerVolume{}

	knownVolumes := s.getVolumeList()
	for _, v := range knownVolumes {
		volName := v[OPT_VOLUME_NAME]
		if volName == "" {
			continue
		}

		mountPoint := v["MountPoint"]

		dv := &DockerVolume{
			Name:       volName,
			Mountpoint: mountPoint,
		}
		vols = append(vols, dv)
	}

	response := pluginResponse{
		Volumes: vols,
	}

	log.Debugf("Successfully got volume list for docker.")

	writeResponseOutput(w, response)
}
