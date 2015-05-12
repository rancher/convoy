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
				Name:  "volume-uuid",
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
				Name:  "volume-uuid",
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
				Name:  "volume-uuid",
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
				Name:  "volume-uuid",
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
				Name:  "volume-uuid",
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

func getVolumeCfgName(uuid string) (string, error) {
	if uuid == "" {
		return "", fmt.Errorf("Invalid volume UUID specified: %v", uuid)
	}
	return "volume_" + uuid + ".json", nil
}

func (config *Config) loadVolume(uuid string) *Volume {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return nil
	}
	if !utils.ConfigExists(config.Root, cfgName) {
		return nil
	}
	volume := &Volume{}
	if err := utils.LoadConfig(config.Root, cfgName, volume); err != nil {
		log.Error("Failed to load volume json ", cfgName)
		return nil
	}
	return volume
}

func (config *Config) saveVolume(volume *Volume) error {
	uuid := volume.UUID
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return utils.SaveConfig(config.Root, cfgName, volume)
}

func (config *Config) deleteVolume(uuid string) error {
	cfgName, err := getVolumeCfgName(uuid)
	if err != nil {
		return err
	}
	return utils.RemoveConfig(config.Root, cfgName)
}

func cmdVolumeCreate(c *cli.Context) {
	if err := doVolumeCreate(c); err != nil {
		panic(err)
	}
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
	volumeUUID, err := getLowerCaseFlag(c, "volume-uuid", false, nil)
	if err != nil {
		return err
	}

	uuid := uuid.New()
	if volumeUUID != "" {
		if config.loadVolume(volumeUUID) != nil {
			return fmt.Errorf("Duplicate volume UUID detected!")
		}
		uuid = volumeUUID
	}
	base := "" //Doesn't support base for now
	if err := driver.CreateVolume(uuid, base, size); err != nil {
		return err
	}
	log.Debug("Created volume using ", config.Driver)

	volume := &Volume{
		UUID:       uuid,
		Base:       base,
		Size:       size,
		MountPoint: "",
		FileSystem: "",
		Snapshots:  make(map[string]bool),
	}
	if err := config.saveVolume(volume); err != nil {
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
	uuid, err := getLowerCaseFlag(c, "volume-uuid", true, err)
	if err != nil {
		return err
	}

	if err = driver.DeleteVolume(uuid); err != nil {
		return err
	}
	log.Debug("Deleted volume using ", config.Driver)
	return config.deleteVolume(uuid)
}

func cmdVolumeList(c *cli.Context) {
	if err := doVolumeList(c); err != nil {
		panic(err)
	}
}

func doVolumeList(c *cli.Context) error {
	_, driver, err := loadGlobalConfig(c)
	uuid, err := getLowerCaseFlag(c, "volume-uuid", false, err)
	snapshotUUID, err := getLowerCaseFlag(c, "snapshot-uuid", false, err)
	if err != nil {
		return err
	}

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
	volumeUUID, err := getLowerCaseFlag(c, "volume-uuid", true, err)
	mountPoint, err := getLowerCaseFlag(c, "mountpoint", true, err)
	fs, err := getLowerCaseFlag(c, "fs", true, err)
	if err != nil {
		return err
	}

	option := c.String("option")
	needFormat := c.Bool("format")
	newNS := c.String("switch-ns")

	volume := config.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Mount(driver, volumeUUID, mountPoint, fs, option, needFormat, newNS); err != nil {
		return err
	}
	log.Debugf("Mount %v to %v", volumeUUID, mountPoint)
	volume.MountPoint = mountPoint
	volume.FileSystem = fs
	return config.saveVolume(volume)
}

func cmdVolumeUmount(c *cli.Context) {
	if err := doVolumeUmount(c); err != nil {
		panic(err)
	}
}

func doVolumeUmount(c *cli.Context) error {
	config, driver, err := loadGlobalConfig(c)
	volumeUUID, err := getLowerCaseFlag(c, "volume-uuid", true, err)
	if err != nil {
		return err
	}

	newNS := c.String("switch-ns")

	volume := config.loadVolume(volumeUUID)
	if volume == nil {
		return fmt.Errorf("volume %v doesn't exist", volumeUUID)
	}
	if err := drivers.Unmount(driver, volume.MountPoint, newNS); err != nil {
		return err
	}
	log.Debugf("Unmount %v from %v", volumeUUID, volume.MountPoint)
	volume.MountPoint = ""
	return config.saveVolume(volume)
}
