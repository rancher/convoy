package vfs

import (
	"fmt"
	"github.com/rancherio/volmgr/objectstore"
	"github.com/rancherio/volmgr/util"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type VfsObjectStoreDriver struct {
	ID   string
	Path string
}

const (
	KIND = "vfs"

	VFS_PATH = "vfs.path"

	MAX_CLEANUP_LEVEL = 10
)

func init() {
	objectstore.RegisterDriver(KIND, initFunc)
}

func initFunc(root, cfgName string, config map[string]string) (objectstore.ObjectStoreDriver, error) {
	b := &VfsObjectStoreDriver{}
	if cfgName != "" {
		if util.ConfigExists(root, cfgName) {
			err := util.LoadConfig(root, cfgName, b)
			if err != nil {
				return nil, err
			}
			return b, nil
		} else {
			return nil, fmt.Errorf("Wrong configuration file for VFS objectstore driver")
		}
	}

	//return temporily driver for loading objectstore config
	b.Path = config[VFS_PATH]
	if b.Path == "" {
		return nil, fmt.Errorf("Cannot find required field %v", VFS_PATH)
	}
	if _, err := b.List(""); err != nil {
		return nil, fmt.Errorf("VFS path %v doesn't exist or is not a directory", b.Path)
	}
	return b, nil
}

func (v *VfsObjectStoreDriver) updatePath(path string) string {
	return filepath.Join(v.Path, path)
}

func (v *VfsObjectStoreDriver) preparePath(file string) error {
	if err := os.MkdirAll(filepath.Dir(v.updatePath(file)), os.ModeDir|0700); err != nil {
		return err
	}
	return nil
}

func (v *VfsObjectStoreDriver) FinalizeInit(root, cfgName, id string) error {
	v.ID = id
	if err := util.SaveConfig(root, cfgName, v); err != nil {
		return err
	}
	return nil
}

func (v *VfsObjectStoreDriver) Kind() string {
	return KIND
}

func (v *VfsObjectStoreDriver) FileSize(filePath string) int64 {
	file := v.updatePath(filePath)
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		return -1
	}
	return st.Size()
}

func (v *VfsObjectStoreDriver) FileExists(filePath string) bool {
	return v.FileSize(filePath) >= 0
}

func (v *VfsObjectStoreDriver) Remove(names ...string) error {
	for _, name := range names {
		if err := os.RemoveAll(v.updatePath(name)); err != nil {
			return err
		}
		//Also automatically cleanup upper level directories
		dir := v.updatePath(name)
		for i := 0; i < MAX_CLEANUP_LEVEL; i++ {
			dir = filepath.Dir(dir)
			// If directory is not empty, then we don't need to continue
			if err := os.Remove(dir); err != nil {
				break
			}
		}
	}
	return nil
}

func (v *VfsObjectStoreDriver) Read(src string) (io.ReadCloser, error) {
	file, err := os.Open(v.updatePath(src))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (v *VfsObjectStoreDriver) Write(dst string, rs io.ReadSeeker) error {
	tmpFile := dst + ".tmp"
	if v.FileExists(tmpFile) {
		v.Remove(tmpFile)
	}
	if err := v.preparePath(dst); err != nil {
		return err
	}
	file, err := os.Create(v.updatePath(tmpFile))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, rs)
	if err != nil {
		return err
	}

	if v.FileExists(dst) {
		v.Remove(dst)
	}
	return os.Rename(v.updatePath(tmpFile), v.updatePath(dst))
}

func (v *VfsObjectStoreDriver) List(path string) ([]string, error) {
	out, err := exec.Command("ls", "-1", v.updatePath(path)).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(string(out))
	}
	var result []string
	if len(out) == 0 {
		return result, nil
	}
	result = strings.Split(strings.TrimSpace(string(out)), "\n")
	return result, nil
}

func (v *VfsObjectStoreDriver) Upload(src, dst string) error {
	tmpDst := dst + ".tmp"
	if v.FileExists(tmpDst) {
		v.Remove(tmpDst)
	}
	if err := v.preparePath(dst); err != nil {
		return err
	}
	output, err := exec.Command("cp", src, v.updatePath(tmpDst)).CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(output))
	}
	output, err = exec.Command("mv", v.updatePath(tmpDst), v.updatePath(dst)).CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(output))
	}
	return nil
}

func (v *VfsObjectStoreDriver) Download(src, dst string) error {
	output, err := exec.Command("cp", v.updatePath(src), dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf(string(output))
	}
	return nil
}
