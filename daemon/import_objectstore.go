package daemon

import (
	// Involve S3 objecstore drivers for registeration
	_ "github.com/rancher/convoy/s3"
	// Involve VFS convoy driver/objectstore driver for registeration
	_ "github.com/rancher/convoy/vfs"
)
