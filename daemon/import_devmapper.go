// +build linux

package daemon

import (
	// Involve device mapper driver for registeration
	_ "github.com/rancher/convoy/devmapper"
)
