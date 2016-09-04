package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"reflect"
	"strings"

	"github.com/codegangsta/cli"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
)

var (
	initCmd = cli.Command{
		Name:   "init",
		Usage:  "ensure convoy daemon exist",
		Action: cmdCheckConvoyDaemon,
	}

	volumeAttachCmd = cli.Command{
		Name:   "attach",
		Usage:  "attach a volume: attach <json-options>",
		Action: cmdAttachVolume,
	}

	volumeDetachCmd = cli.Command{
		Name:   "detach",
		Usage:  "detach a volume: detach <device>",
		Action: cmdDetachVolume,
	}

	mountCmd = cli.Command{
		Name:   "mount",
		Usage:  "mount a volume: mount <mountpoint> <device> <json-options>",
		Action: cmdMount,
	}

	unmountCmd = cli.Command{
		Name:   "unmount",
		Usage:  "unmount a volume: unmount <mountpoint>",
		Action: cmdUnmount,
	}
)

func NewK8sCli(version string) *cli.App {
	app := cli.NewApp()
	app.Name = "k8s"
	app.Version = version
	app.Author = "rancherlabs"
	app.Usage = "A kubernetes volume driver"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "socket, s",
			Value: "/var/run/convoy/convoy.sock",
			Usage: "Specify unix domain socket for communication between server and client",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "Enable debug level log with client or not",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "Verbose level output for client, for create volume/snapshot etc",
		},
	}
	app.CommandNotFound = cmdNotFound
	app.Before = initClient
	app.Commands = []cli.Command{
		initCmd,
		volumeAttachCmd,
		volumeDetachCmd,
		mountCmd,
		unmountCmd,
	}
	return app
}

func cmdCheckConvoyDaemon(c *cli.Context) {
	// ensure daemon is running
	if _, _, err := client.call("GET", "/info", nil, nil); err != nil {
		fmt.Print("{\"status\": \"Failure\"}")
		panic(err)
	}
	fmt.Print("{\"status\": \"Success\"}")
}

func cmdAttachVolume(c *cli.Context) {
	if err := doAttachVolume(c); err != nil {
		fmt.Print("{\"status\": \"Failure\"}")
		panic(err)
	}
}

func doAttachVolume(c *cli.Context) error {
	var err error

	jsonOptions := c.Args().First()
	log.Debugf("jsonOptions: %s", jsonOptions)

	// parse json options:
	//
	// driver:      driver name, such as NFS, EBS, EFS.
	// readOnly:    the volume mounted as readOnly
	// fsType:      the volume formated as this file system type, if volume is created new
	//
	// Following parameters are for EBS:
	// volumeId:    existing EBS volume id
	// size:  		create a new volume using this size
	// volumeType:  create a new volume using this volume type
	// iops:		create a new volume using this iops
	//
	// Following parameters are for NFS or EFS:
	// name:		folder name or volume name in Convoy VFS driver

	optionsMap := make(map[string]string)
	err = json.Unmarshal([]byte(jsonOptions), &optionsMap)
	if err != nil {
		return err
	}
	log.Debugf("optionsMap: %v", optionsMap)

	driverName, ok := optionsMap["convoyDriver"]
	if !ok {
		return fmt.Errorf("no convoyDriver option specified")
	}
	request := &api.VolumeCreateRequest{}

	switch driverName {
	case "ebs":
		request.DriverName = driverName
		return doAttachEBSVolume(request, optionsMap)
	case "efs":
		fallthrough
	case "nfs":
		request.DriverName = "vfs"
		return doCreateVFSVolume(request, optionsMap)
	default:
		return fmt.Errorf("unrecognized convoyDriver name specified")
	}
}

func doAttachEBSVolume(request *api.VolumeCreateRequest, optionsMap map[string]string) error {
	request.DriverVolumeID = optionsMap["volumeID"]
	request.Type = optionsMap["volumeType"]

	size, err := util.ParseSize(optionsMap["size"])
	if err != nil {
		return err
	}
	request.Size = size
	iops, err := util.ParseSize(optionsMap["iops"])
	if err != nil {
		return err
	}
	request.IOPS = iops
	request.Verbose = true // need attached device name from the response
	request.FSType = optionsMap["kubernetes.io/fsType"]

	url := "/volumes/create"
	rc, err := sendRequest("POST", url, request)
	if err != nil {
		return err
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return err
	}
	log.Debugf("response: %s\n", string(b))
	var vol api.VolumeResponse
	err = json.Unmarshal(b, &vol)
	if err != nil {
		return err
	}
	device := vol.DriverInfo["Device"]
	fmt.Printf("{\"status\": \"Success\", \"device\":\"%s\"}", device)

	return nil
}

func cmdDetachVolume(c *cli.Context) {
	device := c.Args().First()
	log.Debugf("device: %s", device)

	if err := doVolumeDetach(device); err != nil {
		fmt.Print("{\"status\": \"Failure\"}")
		panic(err)
	}
	fmt.Print("{\"status\": \"Success\"}")
}

func doVolumeDetach(device string) error {
	var vol *api.VolumeResponse
	var err error

	if strings.HasPrefix(device, "/dev") { // block device
		vol, err = getVolumeByProperty("Device", device)
		if err != nil {
			return err
		}
	} else if strings.ContainsAny(device, ":") { // networked FS, such as NFS
		lastSlashIndex := strings.LastIndex(device, "/")
		name := device[lastSlashIndex+1:]
		vol, err = getVolumeByProperty("VolumeName", name)
		if err != nil {
			return err
		}
	}

	switch vol.Driver {
	case "vfs":
		fallthrough
	case "ebs":
		request := &api.VolumeDeleteRequest{
			VolumeName: vol.Name,
		}

		url := "/volumes/"
		_, err := sendRequest("DELETE", url, request)
		if err != nil {
			return fmt.Errorf("Error deleting " + vol.Name + ": " + err.Error())
		}
		return nil
	default:
		return nil
	}
}

func getVolumeByProperty(key string, value string) (*api.VolumeResponse, error) {
	vol, err := findVolumeByProperty(key, value)
	if err != nil {
		return nil, err
	}
	if vol == nil {
		return nil, fmt.Errorf("can't find volume by DriverInfo property: key=%s, value=%s", key, value)
	}

	return vol, nil
}

func findVolumeByProperty(key string, value string) (*api.VolumeResponse, error) {
	v := url.Values{}
	url := "/volumes/list?" + v.Encode()
	rc, err := sendRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	resp := make(map[string]api.VolumeResponse)
	log.Debugf("volume list response: %s\n", string(b))
	err = json.Unmarshal(b, &resp)
	if err != nil {
		return nil, err
	}

	// loop through and find the volume
	var volume api.VolumeResponse
	for _, vol := range resp {
		propertyValue, ok := vol.DriverInfo[key]
		if !ok {
			continue
		}
		if propertyValue == value {
			volume = vol
			break
		}
	}
	if reflect.DeepEqual(volume, api.VolumeResponse{}) {
		return nil, nil
	}

	return &volume, nil
}

func doCreateVFSVolume(request *api.VolumeCreateRequest, optionsMap map[string]string) error {
	name, ok := optionsMap["name"]
	if !ok {
		return fmt.Errorf("no name option specified")
	}

	// check if the volume exists or not. If exists, then do nothing
	vol, err := findVolumeByProperty("VolumeName", name)
	if err != nil {
		return err
	}
	if vol == nil {
		request.Name = name

		url := "/volumes/create"
		_, err := sendRequest("POST", url, request)
		if err != nil {
			return err
		}
	}
	fmt.Printf("{\"status\": \"Success\"}")

	return nil
}

func cmdMount(c *cli.Context) {
	if err := doMount(c); err != nil {
		fmt.Print("{\"status\": \"Failure\"}")
		panic(err)
	}
	fmt.Print("{\"status\": \"Success\"}")
}

func doMount(c *cli.Context) error {
	var err error
	mountpoint := c.Args().First()
	tail := c.Args().Tail()
	jsonOptions := tail[len(tail)-1]

	log.Debugf("mountpoint: %s, jsonOptions: %s", mountpoint, jsonOptions)

	// parse json options:
	//
	// driver:      driver name, such as NFS, EBS, EFS.
	// readOnly:    the volume mounted as readOnly
	// fsType:      the volume formated as this file system type, if volume is created new
	//
	// Following parameters are for EBS:
	// volumeId:    existing EBS volume id
	// size:  		create a new volume using this size
	// volumeType:  create a new volume using this volume type
	// iops:		create a new volume using this iops
	//
	// Following parameters are for NFS or EFS:
	// name:		folder name or volume name in Convoy VFS driver

	optionsMap := make(map[string]string)
	err = json.Unmarshal([]byte(jsonOptions), &optionsMap)
	if err != nil {
		return err
	}
	log.Debugf("optionsMap: %v", optionsMap)

	driverName, ok := optionsMap["convoyDriver"]
	if !ok {
		return fmt.Errorf("no convoyDriver option specified")
	}

	request := &api.VolumeMountRequest{}

	// k8s needs driver to create mountpoint directory, but k8s will delete it when unmount
	if err := util.CallMkdirIfNotExists(mountpoint); err != nil {
		return err
	}
	request.MountPoint = mountpoint
	request.ReadWrite = optionsMap["kubernetes.io/readwrite"]
	log.Debugf("kubernetes.io/readwrite: %s", request.ReadWrite)

	if request.ReadWrite != "rw" && request.ReadWrite != "ro" {
		return fmt.Errorf("kubernetes.io/readwrite is not rw or ro")
	}
	switch driverName {
	case "ebs":
		device := c.Args().Get(1)
		log.Debugf("device: %s", device)
		return doMountEBS(request, device, optionsMap)
	case "efs":
		fallthrough
	case "nfs":
		return doMountVFS(request, optionsMap)
	default:
		return fmt.Errorf("unrecognized convoyDriver name specified")
	}
}

func doMountEBS(request *api.VolumeMountRequest, device string, optionsMap map[string]string) error {
	vol, err := getVolumeByProperty("Device", device)
	if err != nil {
		return err
	}
	request.VolumeName = vol.Name

	url := "/volumes/mount"
	_, err = sendRequest("POST", url, request)
	if err != nil {
		return fmt.Errorf("Error mounting device: %s to mountpoint: %s, err: %s", device, request.MountPoint, err)
	}

	return nil
}

func doMountVFS(request *api.VolumeMountRequest, optionsMap map[string]string) error {
	name, ok := optionsMap["name"]
	if !ok {
		return fmt.Errorf("no name option specified")
	}
	request.VolumeName = name
	request.BindMount = "rbind"
	request.ReMount = true

	url := "/volumes/mount"
	if _, err := sendRequest("POST", url, request); err != nil {
		return fmt.Errorf("Error bind mounting: %s to mountpoint: %s, err: %s", name, request.MountPoint, err)
	}

	return nil
}

func cmdUnmount(c *cli.Context) {
	if err := doUnmount(c); err != nil {
		fmt.Print("{\"status\": \"Failure\"}")
		panic(err)
	}
	fmt.Print("{\"status\": \"Success\"}")
}

func doUnmount(c *cli.Context) error {
	mountpoint := c.Args().First()
	vol, err := getVolumeByProperty("MountPoint", mountpoint)
	if err != nil {
		return err
	}

	request := &api.VolumeUmountRequest{
		VolumeName: vol.Name,
	}
	url := "/volumes/umount"
	_, err = sendRequest("POST", url, request)
	if err != nil {
		return fmt.Errorf("Error unmounting mountpoint: %s, err: %s", mountpoint, err)
	}

	return nil
}
