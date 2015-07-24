package client

import (
	"encoding/json"
	"fmt"
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/util"
	"net/url"
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
				Name:  "backup",
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

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
}

func getSize(c *cli.Context, err error) (int64, error) {
	size, err := util.GetLowerCaseFlag(c, "size", false, err)
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
	backupURL, err := util.GetLowerCaseFlag(c, "backup", false, err)
	if err != nil {
		return err
	}

	if backupURL != "" && size != 0 {
		return fmt.Errorf("Cannot specify volume size with backup-url. It would be the same size of backup")
	}

	request := &api.VolumeCreateRequest{
		Name:      name,
		Size:      size,
		BackupURL: backupURL,
	}

	url := "/volumes/create"

	return sendRequestAndPrint("POST", url, request)
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
		id, err = util.GetLowerCaseFlag(c, key, required, err)
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
	v.Set(api.KEY_NAME, id)

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

	url := "/volumes/" + uuid + "/"

	return sendRequestAndPrint("DELETE", url, nil)
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

	url := "/volumes/list" + v.Encode()
	return sendRequestAndPrint("GET", url, nil)
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

	url := "/volumes/" + volumeUUID + "/"
	return sendRequestAndPrint("GET", url, nil)
}

func cmdVolumeMount(c *cli.Context) {
	if err := doVolumeMount(c); err != nil {
		panic(err)
	}
}

func doVolumeMount(c *cli.Context) error {
	var err error

	volumeUUID, err := getOrRequestUUID(c, "", true)
	mountPoint, err := util.GetLowerCaseFlag(c, "mountpoint", false, err)
	if err != nil {
		return err
	}

	request := api.VolumeMountRequest{
		MountPoint: mountPoint,
	}

	url := "/volumes/" + volumeUUID + "/mount"
	return sendRequestAndPrint("POST", url, request)
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

	url := "/volumes/" + volumeUUID + "/umount"
	return sendRequestAndPrint("POST", url, nil)
}
