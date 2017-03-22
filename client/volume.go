package client

import (
	"fmt"
	"net/url"

	"github.com/codegangsta/cli"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
)

var (
	volumeCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a new volume: create [volume_name] [options]",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "driver",
				Usage: "specify using driver other than default",
			},
			cli.StringFlag{
				Name:  "size",
				Usage: "size of volume if driver supports, in bytes, or end in either G or M or K",
			},
			cli.StringFlag{
				Name:  "backup",
				Usage: "create a volume of backup if driver supports",
			},
			cli.StringFlag{
				Name:  "id",
				Usage: "driver specific volume ID if driver supports",
			},
			cli.StringFlag{
				Name:  "type",
				Usage: "driver specific volume type if driver supports",
			},
			cli.StringFlag{
				Name:  "iops",
				Usage: "IOPS if driver supports",
			},
			cli.BoolFlag{
				Name:  "vm",
				Usage: "Prepare volume for Rancher VM if driver supports",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd = cli.Command{
		Name:  "delete",
		Usage: "delete a volume: delete <volume> [options]",
		Flags: []cli.Flag{
			cli.BoolFlag{
				Name:  "reference, r",
				Usage: "only delete the reference of volume if driver supports",
			},
		},
		Action: cmdVolumeDelete,
	}

	volumeMountCmd = cli.Command{
		Name:  "mount",
		Usage: "mount a volume: mount <volume> [options]",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "mountpoint",
				Usage: "mountpoint of volume. If not specified, it would be automatic mounted to default directory",
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
			cli.StringFlag{
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
	size, err := util.GetFlag(c, "size", false, err)
	if err != nil {
		return 0, err
	}
	return util.ParseSize(size)
}

func doVolumeCreate(c *cli.Context) error {
	var err error

	name := c.Args().First()
	size, err := getSize(c, err)
	driverName, err := util.GetFlag(c, "driver", false, err)
	backupURL, err := util.GetFlag(c, "backup", false, err)
	if err != nil {
		return err
	}

	driverVolumeID := c.String("id")
	volumeType := c.String("type")
	iops := c.Int("iops")
	prepareForVM := c.Bool("vm")

	request := &api.VolumeCreateRequest{
		Name:           name,
		DriverName:     driverName,
		Size:           size,
		BackupURL:      backupURL,
		DriverVolumeID: driverVolumeID,
		Type:           volumeType,
		IOPS:           int64(iops),
		PrepareForVM:   prepareForVM,
		Verbose:        c.GlobalBool(verboseFlag),
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

	names, err := getNames(c)
	if err != nil {
		return err
	}

	for _, name := range names {
		request := &api.VolumeDeleteRequest{
			VolumeName:    name,
			ReferenceOnly: c.Bool("reference"),
		}

		url := "/volumes/"

		reqErr := sendRequestAndPrint("DELETE", url, request)
		if reqErr != nil {
			err = reqErr
			fmt.Println("Error deleting " + name + ": " + reqErr.Error())
		}
	}

	return err
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	v := url.Values{}

        var err error
        driver, err := util.GetFlag(c, "driver", false, err)
        if err != nil {
            return err
        }
        if driver != "" {
            v.Set("driver", driver)
        }

	url := "/volumes/list?" + v.Encode()
	return sendRequestAndPrint("GET", url, nil)
}

func cmdVolumeInspect(c *cli.Context) {
	if err := doVolumeInspect(c); err != nil {
		panic(err)
	}
}

func doVolumeInspect(c *cli.Context) error {
	var err error

	volumeName, err := getName(c, "", true)
	if err != nil {
		return err
	}

	request := &api.VolumeInspectRequest{
		VolumeName: volumeName,
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

	volumeName, err := getName(c, "", true)
	mountPoint, err := util.GetFlag(c, "mountpoint", false, err)
	if err != nil {
		return err
	}

	request := &api.VolumeMountRequest{
		VolumeName: volumeName,
		MountPoint: mountPoint,
		Verbose:    c.GlobalBool(verboseFlag),
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

	volumeName, err := getName(c, "", true)
	if err != nil {
		return err
	}

	request := &api.VolumeUmountRequest{
		VolumeName: volumeName,
	}
	url := "/volumes/umount"
	return sendRequestAndPrint("POST", url, request)
}
