package client

import (
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

func doVolumeDelete(c *cli.Context) error {
	var err error

	uuid, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	request := &api.VolumeDeleteRequest{
		VolumeUUID: uuid,
	}

	url := "/volumes/"

	return sendRequestAndPrint("DELETE", url, request)
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

	request := &api.VolumeInspectRequest{
		VolumeUUID: volumeUUID,
	}
	url := "/volumes/"
	return sendRequestAndPrint("GET", url, request)
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

	request := &api.VolumeMountRequest{
		VolumeUUID: volumeUUID,
		MountPoint: mountPoint,
	}

	url := "/volumes/mount"
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

	request := &api.VolumeUmountRequest{
		VolumeUUID: volumeUUID,
	}
	url := "/volumes/umount"
	return sendRequestAndPrint("POST", url, request)
}
