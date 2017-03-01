package flags

import (
	"github.com/codegangsta/cli"
)

var (
	DaemonFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Debug log, enabled by default",
		},
		cli.StringFlag{
			Name:  "log",
			Usage: "specific output log file, otherwise output to stdout by default",
		},
		cli.StringFlag{
			Name:  "root",
			Value: "/var/lib/rancher/convoy",
			Usage: "specific root directory of convoy, if configure file exists, daemon specific options would be ignored",
		},
		cli.StringFlag{
			Name:  "config",
			Usage: "Config filename for driver",
		},
		cli.StringFlag{
			Name:  "mnt-ns",
			Usage: "Specify mount namespace file descriptor if user don't want to mount in current namespace. Support by Device Mapper and EBS",
		},
		cli.BoolFlag{
			Name:  "ignore-docker-delete",
			Usage: "Do not delete volumes when told to by Docker",
		},
		cli.BoolFlag{
			Name:  "create-on-docker-mount",
			Usage: "Create a volume if docker asks to do a mount and the volume doesn't exist.",
		},
		cli.StringFlag{
			Name:  "cmd-timeout",
			Usage: "Set timeout value for executing each command. One minute (1m) by default and at least one minute.",
		},
		cli.BoolFlag{
			Name:  "ignore-config-file",
			Usage: "Avoid loading the existing config file when starting daemon, and use the command line options instead (not including driver options)",
		},
	}
)
