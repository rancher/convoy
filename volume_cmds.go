package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/utils"
)

var (
	volumeCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a new volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.IntFlag{
				Name:  "size",
				Usage: "size of volume, in bytes",
			},
		},
		Action: cmdVolumeCreate,
	}

	volumeDeleteCmd = cli.Command{
		Name:  "delete",
		Usage: "delete a volume with all of it's snapshots",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
		},
		Action: cmdVolumeDelete,
	}

	volumeMountCmd = cli.Command{
		Name:  "mount",
		Usage: "mount a volume to an specific path",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "mountpoint",
				Usage: "mountpoint of volume",
			},
			cli.StringFlag{
				Name:  "fs",
				Value: "ext4",
				Usage: "filesystem of volume(supports ext4 only)",
			},
			cli.BoolFlag{
				Name:  "format",
				Usage: "format or not",
			},
			cli.StringFlag{
				Name:  "option",
				Usage: "mount options",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeMount,
	}

	volumeUmountCmd = cli.Command{
		Name:  "umount",
		Usage: "umount a volume",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume",
			},
			cli.StringFlag{
				Name:  "switch-ns",
				Usage: "switch to another mount namespace, need namespace file descriptor",
			},
		},
		Action: cmdVolumeUmount,
	}

	volumeListCmd = cli.Command{
		Name:  "list",
		Usage: "list all managed volumes",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "uuid",
				Usage: "uuid of volume, if not supplied, would list all volumes",
			},
			cli.StringFlag{
				Name:  "snapshot-uuid",
				Usage: "uuid of snapshot, must be used with volume uuid",
			},
		},
		Action: cmdVolumeList,
	}

	volumeCmd = cli.Command{
		Name:  "volume",
		Usage: "volume related operations",
		Subcommands: []cli.Command{
			volumeCreateCmd,
			volumeDeleteCmd,
			volumeMountCmd,
			volumeUmountCmd,
			volumeListCmd,
		},
	}
)

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
}

func duplicateVolumeUUID(config *Config, uuid string) bool {
	_, exists := config.Volumes[uuid]
	return exists
}

func doVolumeCreate(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}

	size := int64(c.Int("size"))
	if size == 0 {
		return genRequiredMissingError("size")
	}
	volumeUUID := c.String("uuid")

	uuid := uuid.New()
	if volumeUUID != "" {
		if duplicateVolumeUUID(config, uuid) {
			return fmt.Errorf("Duplicate volume UUID detected!")
		}
		uuid = volumeUUID
	}
	base := "" //Doesn't support base for now
	if err := driver.CreateVolume(uuid, base, size); err != nil {
		return err
	}
	log.Debug("Created volume using ", config.Driver)
	config.Volumes[uuid] = Volume{
		Base:       base,
		Size:       size,
		MountPoint: "",
		FileSystem: "",
		Snapshots:  make(map[string]bool),
	}
	if err := utils.SaveConfig(config.Root, getCfgName(), config); err != nil {
		return err
	}
	api.ResponseOutput(api.VolumeResponse{
		UUID: uuid,
		Base: base,
		Size: size,
	})
	return nil
}

func cmdVolumeDelete(c *cli.Context) {
	if err := doVolumeDelete(c); err != nil {
		panic(err)
	}
}

func doVolumeDelete(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	if uuid == "" {
		return genRequiredMissingError("uuid")
	}

	if err := driver.DeleteVolume(uuid); err != nil {
		return err
	}
	log.Debug("Deleted volume using ", config.Driver)
	delete(config.Volumes, uuid)
	return utils.SaveConfig(config.Root, getCfgName(), config)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	_, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	uuid := c.String("uuid")
	snapshotUUID := c.String("snapshot-uuid")
	if snapshotUUID != "" && uuid == "" {
		return fmt.Errorf("--snapshot-uuid must be used with volume uuid")
	}
	return driver.ListVolume(uuid, snapshotUUID)
}

func cmdVolumeMount(c *cli.Context) {
	if err := doVolumeMount(c); err != nil {
		panic(err)
	}
}

func doVolumeMount(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	volumeUUID := c.String("uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("uuid")
	}
	mountPoint := c.String("mountpoint")
	if mountPoint == "" {
		return genRequiredMissingError("mountpoint")
	}
	fs := c.String("fs")
	if fs == "" {
		return genRequiredMissingError("fs")
	}

	option := c.String("option")
	needFormat := c.Bool("format")
	newNS := c.String("switch-ns")

	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Mount(driver, volumeUUID, mountPoint, fs, option, needFormat, newNS); err != nil {
		return err
	}
	log.Debugf("Mount %v to %v", volumeUUID, mountPoint)
	volume.MountPoint = mountPoint
	volume.FileSystem = fs
	config.Volumes[volumeUUID] = volume
	return utils.SaveConfig(config.Root, getCfgName(), config)
}

func cmdVolumeUmount(c *cli.Context) {
	if err := doVolumeUmount(c); err != nil {
		panic(err)
	}
}

func doVolumeUmount(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	if err != nil {
		return err
	}
	volumeUUID := c.String("uuid")
	if volumeUUID == "" {
		return genRequiredMissingError("uuid")
	}
	newNS := c.String("switch-ns")

	volume, exists := config.Volumes[volumeUUID]
	if !exists {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Unmount(driver, volume.MountPoint, newNS); err != nil {
		return err
	}
	log.Debugf("Unmount %v from %v", volumeUUID, volume.MountPoint)
	volume.MountPoint = ""
	config.Volumes[volumeUUID] = volume
	return utils.SaveConfig(config.Root, getCfgName(), config)
}
