package fs

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

var ErrNoFilesystemDetected = errors.New("no filesystem detected")

func FormatDevice(devicePath string, fsType string) error {
	switch fsType {
	case "btrfs", "ext2", "ext3", "ext4", "minix", "xfs":
	default:
		return fmt.Errorf("unrecognized or unsupported fs-type: %s", fsType)
	}
	cmd := exec.Command("sh", "-c", fmt.Sprintf("yes | sudo -n mkfs.%s %s", fsType, devicePath))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("FormatDevice: %s: %s", string(output), err)
	}
	return nil
}

// Detect determines the filesystem type for the given device.
// An empty-string return indicates an unformatted device.
func Detect(devicePath string) (string, error) {
	cmd := exec.Command("sudo", "-n", "blkid", "-s", "TYPE", "-o", "value", devicePath)
	output, err := cmd.CombinedOutput()
	output = bytes.Trim(output, "\r\n \t")
	if err != nil {
		if len(output) == 0 && err.Error() == "exit status 2" {
			// Then no filesystem detected.
			return "", ErrNoFilesystemDetected
		}
		return "", fmt.Errorf("Detect: %s: %s", string(output), err)
	}
	fsType := string(output)
	return fsType, nil
}
