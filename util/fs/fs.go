package fs

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
)

var (
	ErrUnrecognizedFilesystemType = errors.New("unrecognized or unsupported filesystem type")
	ErrNoFilesystemDetected       = errors.New("no filesystem detected")
	ErrResizeNotAvailable         = errors.New("resize function not available for the filesystem type of this volume")
)

func FormatDevice(devicePath string, fsType string) error {
	switch fsType {
	case "btrfs", "ext2", "ext3", "ext4", "minix", "xfs":
	default:
		return ErrUnrecognizedFilesystemType
	}
	cmd, err := sudoCmd("sh", "-c", fmt.Sprintf("set -e && yes | mkfs.%v %v", fsType, devicePath))
	if err != nil {
		return err
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("FormatDevice: %v: %s", string(output), err)
	}
	return nil
}

// Detect determines the filesystem type for the given device.
// An empty-string return indicates an unformatted device.
func Detect(devicePath string) (string, error) {
	cmd, err := sudoCmd("blkid", "-s", "TYPE", "-o", "value", devicePath)
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	output = bytes.Trim(output, "\r\n \t")
	if err != nil {
		if len(output) == 0 && err.Error() == "exit status 2" {
			// Then no filesystem detected.
			return "", ErrNoFilesystemDetected
		}
		return "", fmt.Errorf("Detect: %v: %s", string(output), err)
	}
	fsType := string(output)
	return fsType, nil
}

// Resize a device path by calling resize2fs on it. In case of success,
// resize2fs only runs a resize when it is
// required on the device; otherwise, it just exits with a code 0 and a message.
func Resize(devicePath string) error {
	fsType, err := Detect(devicePath)
	if err != nil {
		return err
	}

	switch fsType {
	case "ext2", "ext3", "ext4":
	default:
		return ErrResizeNotAvailable
	}

	cmd, err := sudoCmd("resize2fs", "-f", devicePath)
	if err != nil {
		return err
	}
	output, err := cmd.CombinedOutput()
	output = bytes.Trim(output, "\r\n \t")
	if err != nil {
		return fmt.Errorf("Resize: %v: %v", devicePath, string(output))
	}
	return nil
}

func sudoCmd(name string, args ...string) (*exec.Cmd, error) {
	prefix, err := sudoCmdPrefix()
	if err != nil {
		return nil, err
	}
	args = append(prefix, append([]string{name}, args...)...)
	cmd := exec.Command(args[0], args[1:]...)
	return cmd, nil
}

// sudoCmdPrefix accounts for sudo only being necessary when UID != 0 (i.e. when not
// root).
func sudoCmdPrefix() ([]string, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if u.Uid != "0" {
		return []string{"sudo", "-n"}, nil
	}
	return nil, nil
}
