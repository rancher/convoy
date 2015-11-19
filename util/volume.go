package util

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

const (
	MOUNT_BINARY   = "mount"
	UMOUNT_BINARY  = "umount"
	NSENTER_BINARY = "nsenter"

	IMAGE_FILE_NAME = "disk.img"

	FILE_TYPE_REGULAR     = "regular file"
	FILE_TYPE_DIRECTORY   = "directory"
	FILE_TYPE_BLOCKDEVICE = "block special file"
)

var (
	mountNamespaceFD = ""
)

/* Caller must implement VolumeHelper interface, and must have fields "UUID" and "MountPoint" */
type VolumeHelper interface {
	GetDevice() (string, error)
	GetMountOpts() []string
	GenerateDefaultMountPoint() string
}

func getFieldString(obj interface{}, field string) (string, error) {
	if reflect.TypeOf(obj).Kind() != reflect.Ptr {
		return "", fmt.Errorf("BUG: Non-pointer was passed in")
	}
	t := reflect.TypeOf(obj).Elem()
	if _, found := t.FieldByName(field); !found {
		return "", fmt.Errorf("BUG: %v doesn't have required field %v", t, field)
	}
	return reflect.ValueOf(obj).Elem().FieldByName(field).String(), nil
}

func setFieldString(obj interface{}, field string, value string) error {
	if reflect.TypeOf(obj).Kind() != reflect.Ptr {
		return fmt.Errorf("BUG: Non-pointer was passed in")
	}
	t := reflect.TypeOf(obj).Elem()
	if _, found := t.FieldByName(field); !found {
		return fmt.Errorf("BUG: %v doesn't have required field %v", t, field)
	}
	v := reflect.ValueOf(obj).Elem().FieldByName(field)
	if !v.CanSet() {
		return fmt.Errorf("BUG: %v doesn't have setable field %v", t, field)
	}
	v.SetString(value)
	return nil
}

func getVolumeUUID(v VolumeHelper) string {
	// We should already pass the test in getVolumeOps
	value, err := getFieldString(v, "UUID")
	if err != nil {
		panic(err)
	}
	return value
}

func getVolumeMountPoint(v VolumeHelper) string {
	// We should already pass the test in getVolumeOps
	value, err := getFieldString(v, "MountPoint")
	if err != nil {
		panic(err)
	}
	return value
}

func setVolumeMountPoint(v VolumeHelper, value string) {
	// We should already pass the test in getVolumeOps
	if err := setFieldString(v, "MountPoint", value); err != nil {
		panic(err)
	}
}

func getVolumeOps(obj interface{}) (VolumeHelper, error) {
	var err error
	if reflect.TypeOf(obj).Kind() != reflect.Ptr {
		return nil, fmt.Errorf("BUG: Non-pointer was passed in")
	}
	_, err = getFieldString(obj, "UUID")
	if err != nil {
		return nil, err
	}
	mountpoint, err := getFieldString(obj, "MountPoint")
	if err != nil {
		return nil, err
	}
	if err = setFieldString(obj, "MountPoint", mountpoint); err != nil {
		return nil, err
	}
	t := reflect.TypeOf(obj).Elem()
	ops, ok := obj.(VolumeHelper)
	if !ok {
		return nil, fmt.Errorf("BUG: %v doesn't implement necessary methods for accessing volume", t)
	}
	return ops, nil
}

func isMounted(mountPoint string) bool {
	output, err := callMount([]string{}, []string{})
	if err != nil {
		return false
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, mountPoint) {
			return true
		}
	}
	return false
}

func VolumeMount(v interface{}, mountPoint string, remount bool) (string, error) {
	vol, err := getVolumeOps(v)
	if err != nil {
		return "", err
	}
	dev, err := vol.GetDevice()
	if err != nil {
		return "", err
	}
	opts := vol.GetMountOpts()
	createMountpoint := false
	if mountPoint == "" {
		mountPoint = vol.GenerateDefaultMountPoint()
		// Create of directory cannot be done before umount, because it
		// won't be recognized as a directory file when mounted sometime
		createMountpoint = true
	}
	existMount := getVolumeMountPoint(vol)
	if existMount != "" && existMount != mountPoint {
		return "", fmt.Errorf("Volume %v was already mounted at %v, but asked to mount at %v", getVolumeUUID(vol), existMount, mountPoint)
	}
	if remount && isMounted(mountPoint) {
		log.Debugf("Umount existing mountpoint %v", mountPoint)
		if err := callUmount([]string{mountPoint}); err != nil {
			return "", err
		}
	}
	if createMountpoint {
		if err := callMkdirIfNotExists(mountPoint); err != nil {
			return "", err
		}
	}
	if !isMounted(mountPoint) {
		log.Debugf("Volume %v is being mounted it to %v, with option %v", getVolumeUUID(vol), mountPoint, opts)
		_, err = callMount(opts, []string{dev, mountPoint})
		if err != nil {
			return "", err
		}
	}
	setVolumeMountPoint(vol, mountPoint)
	return mountPoint, nil
}

func VolumeUmount(v interface{}) error {
	vol, err := getVolumeOps(v)
	if err != nil {
		return err
	}
	mountPoint := getVolumeMountPoint(vol)
	if mountPoint == "" {
		log.Debugf("Umount a umounted volume %v", getVolumeUUID(vol))
		return nil
	}
	if err := callUmount([]string{mountPoint}); err != nil {
		return err
	}
	if mountPoint == vol.GenerateDefaultMountPoint() {
		if err := os.Remove(mountPoint); err != nil {
			log.Warnf("Cannot cleanup mount point directory %v due to %v\n", mountPoint, err)
		}
	}
	setVolumeMountPoint(vol, "")
	return nil
}

func callMkdirIfNotExists(dirName string) error {
	cmdName := "mkdir"
	cmdArgs := []string{"-p", dirName}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	_, err := Execute(cmdName, cmdArgs)
	if err != nil {
		return err
	}
	return nil
}

func callMount(opts, args []string) (string, error) {
	cmdName := MOUNT_BINARY
	cmdArgs := opts
	cmdArgs = append(cmdArgs, args...)
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	output, err := Execute(cmdName, cmdArgs)
	if err != nil {
		return "", err
	}
	return output, nil
}

func callUmount(args []string) error {
	cmdName := UMOUNT_BINARY
	cmdArgs := args
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	if _, err := Execute(cmdName, cmdArgs); err != nil {
		return err
	}
	return nil
}

func InitMountNamespace(fd string) error {
	if fd == "" {
		return nil
	}
	if _, err := Execute(NSENTER_BINARY, []string{"-V"}); err != nil {
		return fmt.Errorf("Cannot find nsenter for namespace switching")
	}
	if _, err := Execute(NSENTER_BINARY, []string{"--mount=" + fd, "mount"}); err != nil {
		return fmt.Errorf("Invalid mount namespace %v, error %v", fd, err)
	}

	mountNamespaceFD = fd
	log.Debugf("Would mount volume in namespace %v", fd)
	return nil
}

func updateMountNamespace(name string, args []string) (string, []string) {
	if mountNamespaceFD == "" {
		return name, args
	}
	cmdArgs := []string{
		"--mount=" + mountNamespaceFD,
		name,
	}
	cmdArgs = append(cmdArgs, args...)
	cmdName := NSENTER_BINARY
	log.Debugf("Execute in namespace %v: %v %v", mountNamespaceFD, cmdName, cmdArgs)
	return cmdName, cmdArgs
}

func getFileType(file string) (string, error) {
	cmdName := "stat"
	cmdArgs := []string{"-c", "%F", file}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	output, err := Execute(cmdName, cmdArgs)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func getFileSize(file string) (int64, error) {
	cmdName := "stat"
	cmdArgs := []string{"-c", "%s", file}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	output, err := Execute(cmdName, cmdArgs)
	if err != nil {
		return 0, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	if err != nil {
		return 0, err
	}
	return size, nil
}

func VolumeMountPointFileExists(v interface{}, file string, expectType string) bool {
	vol, err := getVolumeOps(v)
	if err != nil {
		panic("BUG: VolumeMountPointDirectoryExists was called with invalid variable")
	}
	mp := getVolumeMountPoint(vol)
	if mp == "" {
		panic("BUG: VolumeMountPointDirectoryExists was called before volume mounted")
	}
	path := filepath.Join(mp, file)

	fileType, err := getFileType(path)
	if err != nil {
		return false
	}

	if fileType == expectType {
		return true
	}
	fmt.Println(fileType, expectType)
	return false
}

func VolumeMountPointDirectoryCreate(v interface{}, dirName string) error {
	vol, err := getVolumeOps(v)
	if err != nil {
		panic("BUG: VolumeMountPointDirectoryCreate was called with invalid variable")
	}
	mp := getVolumeMountPoint(vol)
	if mp == "" {
		panic("BUG: VolumeMountPointDirectoryCreate was called before volume mounted")
	}
	path := filepath.Join(mp, dirName)

	cmdName := "mkdir"
	cmdArgs := []string{"-p", path}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	if _, err := Execute(cmdName, cmdArgs); err != nil {
		return err
	}
	return nil
}

func VolumeMountPointDirectoryRemove(v interface{}, dirName string) error {
	vol, err := getVolumeOps(v)
	if err != nil {
		panic("BUG: VolumeMountPointDirectoryRemove was called with invalid variable")
	}
	mp := getVolumeMountPoint(vol)
	if mp == "" {
		panic("BUG: VolumeMountPointDirectoryRemove was called before volume mounted")
	}
	path := filepath.Join(mp, dirName)

	cmdName := "rm"
	cmdArgs := []string{"-rf", path}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	if _, err := Execute(cmdName, cmdArgs); err != nil {
		return err
	}
	return nil
}

func createImage(file string, size int64) error {
	cmdName := "truncate"
	cmdArgs := []string{
		"-s",
		strconv.FormatInt(size, 10),
		file,
	}
	cmdName, cmdArgs = updateMountNamespace(cmdName, cmdArgs)
	if _, err := Execute(cmdName, cmdArgs); err != nil {
		return err
	}
	return nil
}

func prepareImage(dir string, size int64) error {
	file := filepath.Join(dir, IMAGE_FILE_NAME)
	fileType, err := getFileType(file)
	if err == nil {
		if fileType != "regular file" {
			return fmt.Errorf("The image is already exists at %v, but not a file? It's %v", file, fileType)
		}
		// File already exists, don't need to do anything
		fileSize, err := getFileSize(file)
		if err != nil {
			return err
		}
		if fileSize != size {
			log.Warnf("The existing image file size %v is different from specified size %v", fileSize, size)
		}
		return nil
	}

	if err := createImage(file, size); err != nil {
		return err
	}
	return nil
}

func MountPointPrepareForVM(mp string, size int64) error {
	fileType, err := getFileType(mp)
	if err != nil {
		return err
	}
	if fileType == "directory" {
		if err := prepareImage(mp, size); err != nil {
			return err
		}
	} else if fileType == "block special file" {
		return fmt.Errorf("Haven't support block file yet")
	} else {
		return fmt.Errorf("Invalid file with type '%v' at %v", fileType, mp)
	}
	return nil
}
