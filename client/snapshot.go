package client

import (
	"github.com/codegangsta/cli"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
)

var (
	snapshotCreateCmd = cli.Command{
		Name:  "create",
		Usage: "create a snapshot for certain volume: snapshot create <volume>",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "name",
				Usage: "name of snapshot",
			},
		},
		Action: cmdSnapshotCreate,
	}

	snapshotDeleteCmd = cli.Command{
		Name:   "delete",
		Usage:  "delete a snapshot: snapshot delete <snapshot>",
		Action: cmdSnapshotDelete,
	}

	snapshotInspectCmd = cli.Command{
		Name:   "inspect",
		Usage:  "inspect an snapshot: snapshot inspect <snapshot>",
		Action: cmdSnapshotInspect,
	}

	snapshotCmd = cli.Command{
		Name:  "snapshot",
		Usage: "snapshot related operations",
		Subcommands: []cli.Command{
			snapshotCreateCmd,
			snapshotDeleteCmd,
			snapshotInspectCmd,
		},
	}
)

func cmdSnapshotCreate(c *cli.Context) {
	if err := doSnapshotCreate(c); err != nil {
		panic(err)
	}
}

func doSnapshotCreate(c *cli.Context) error {
	var err error

	volumeUUID, err := getOrRequestUUID(c, "", true)
	snapshotName, err := util.GetName(c, "name", false, err)
	if err != nil {
		return err
	}

	request := &api.SnapshotCreateRequest{
		Name:       snapshotName,
		VolumeUUID: volumeUUID,
		Verbose:    c.GlobalBool(verboseFlag),
	}

	url := "/snapshots/create"

	return sendRequestAndPrint("POST", url, request)
}

func cmdSnapshotDelete(c *cli.Context) {
	if err := doSnapshotDelete(c); err != nil {
		panic(err)
	}
}

func doSnapshotDelete(c *cli.Context) error {
	var err error
	snapshotName, err := getName(c, "", true)
	if err != nil {
		return err
	}

	request := &api.SnapshotDeleteRequest{
		SnapshotName: snapshotName,
	}
	url := "/snapshots/"
	return sendRequestAndPrint("DELETE", url, request)
}

func cmdSnapshotInspect(c *cli.Context) {
	if err := doSnapshotInspect(c); err != nil {
		panic(err)
	}
}

func doSnapshotInspect(c *cli.Context) error {
	var err error

	snapshotName, err := getName(c, "", true)
	if err != nil {
		return err
	}

	request := &api.SnapshotInspectRequest{
		SnapshotName: snapshotName,
	}
	url := "/snapshots/"
	return sendRequestAndPrint("GET", url, request)
}
