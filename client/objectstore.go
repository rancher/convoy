package client

import (
	"github.com/codegangsta/cli"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/util"
)

var (
	backupCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a backup in objectstore: create <snapshot>",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "dest",
				Usage: "required. destination of backup, would be url like s3://bucket@region/path/ or vfs:///path/",
			},
		},
		Action: cmdBackupCreate,
	}

	backupDeleteCmd = cli.Command{
		Name:   "delete",
		Usage:  "delete a backup in objectstore: delete <backup>",
		Action: cmdBackupDelete,
	}

	backupListCmd = cli.Command{
		Name:  "list",
		Usage: "list volume in objectstore: list <dest>",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "volume-uuid",
				Usage: "uuid of volume",
			},
		},
		Action: cmdBackupList,
	}

	backupInspectCmd = cli.Command{
		Name:   "inspect",
		Usage:  "inspect a backup: inspect <backup>",
		Action: cmdBackupInspect,
	}

	backupCmd = cli.Command{
		Name:  "backup",
		Usage: "backup related operations",
		Subcommands: []cli.Command{
			backupCreateCmd,
			backupDeleteCmd,
			backupListCmd,
			backupInspectCmd,
		},
	}
)

func cmdBackupList(c *cli.Context) {
	if err := doBackupList(c); err != nil {
		panic(err)
	}
}

func doBackupList(c *cli.Context) error {
	var err error

	destURL, err := util.GetLowerCaseFlag(c, "", true, err)
	volumeUUID, err := util.GetUUID(c, "volume-uuid", false, err)
	if err != nil {
		return err
	}

	request := &api.BackupListRequest{
		URL:        destURL,
		VolumeUUID: volumeUUID,
	}
	url := "/backups/list"
	return sendRequestAndPrint("GET", url, request)
}

func cmdBackupInspect(c *cli.Context) {
	if err := doBackupInspect(c); err != nil {
		panic(err)
	}
}

func doBackupInspect(c *cli.Context) error {
	var err error

	backupURL, err := util.GetLowerCaseFlag(c, "", true, err)
	if err != nil {
		return err
	}

	request := &api.BackupListRequest{
		URL: backupURL,
	}
	url := "/backups/inspect"
	return sendRequestAndPrint("GET", url, request)
}

func cmdBackupCreate(c *cli.Context) {
	if err := doBackupCreate(c); err != nil {
		panic(err)
	}
}

func doBackupCreate(c *cli.Context) error {
	var err error

	destURL, err := util.GetLowerCaseFlag(c, "dest", true, err)
	if err != nil {
		return err
	}

	snapshotUUID, err := getOrRequestUUID(c, "", true)
	if err != nil {
		return err
	}

	request := &api.BackupCreateRequest{
		URL:          destURL,
		SnapshotUUID: snapshotUUID,
	}

	url := "/backups/create"
	return sendRequestAndPrint("POST", url, request)
}

func cmdBackupDelete(c *cli.Context) {
	if err := doBackupDelete(c); err != nil {
		panic(err)
	}
}

func doBackupDelete(c *cli.Context) error {
	var err error
	backupURL, err := util.GetLowerCaseFlag(c, "", true, err)
	if err != nil {
		return err
	}

	request := &api.BackupDeleteRequest{
		URL: backupURL,
	}
	url := "/backups"
	return sendRequestAndPrint("DELETE", url, request)
}
