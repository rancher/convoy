package vfsblockstore

import (
	"fmt"
	"github.com/rancherio/volmgr/blockstore"
	"github.com/rancherio/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type VfsBlockStoreDriver struct {
	ID   string
	Path string
}

const (
	KIND = "vfs"

	VFS_PATH = "vfs.path"
)

func init() {
	blockstore.RegisterDriver(KIND, initFunc)
}

func initFunc(root, cfgName string, config map[string]string) (blockstore.BlockStoreDriver, error) {
	b := &VfsBlockStoreDriver{}
	if cfgName != "" {
		if utils.ConfigExists(root, cfgName) {
			err := utils.LoadConfig(root, cfgName, b)
			if err != nil {
				return nil, err
			}
			return b, nil
		} else {
			return nil, fmt.Errorf("Wrong configuration file for VFS blockstore driver")
		}
	}

	//return temporily driver for loading blockstore config
	b.Path = config[VFS_PATH]
	if b.Path == "" {
		return nil, fmt.Errorf("Cannot find required field %v", VFS_PATH)
	}
	if st, err := os.Stat(b.Path); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("VFS path %v doesn't exist or is not a directory", b.Path)
	}
	return b, nil
}

func (v *VfsBlockStoreDriver) updatePath(path string) string {
	return filepath.Join(v.Path, path)
}

func (v *VfsBlockStoreDriver) FinalizeInit(root, cfgName, id string) error {
	v.ID = id
	if err := utils.SaveConfig(root, cfgName, v); err != nil {
		return err
	}
	return nil
}

func (v *VfsBlockStoreDriver) Kind() string {
	return KIND
}

func (v *VfsBlockStoreDriver) FileSize(filePath string) int64 {
	file := v.updatePath(filePath)
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		return -1
	}
	return st.Size()
}

func (v *VfsBlockStoreDriver) FileExists(filePath string) bool {
	return v.FileSize(filePath) >= 0
}

func (v *VfsBlockStoreDriver) MkDirAll(dirName string) error {
	return os.MkdirAll(v.updatePath(dirName), os.ModeDir|0700)
}

func (v *VfsBlockStoreDriver) RemoveAll(name string) error {
	return os.RemoveAll(v.updatePath(name))
}

func (v *VfsBlockStoreDriver) Remove(name string) error {
	return os.Remove(v.updatePath(name))
}

func (v *VfsBlockStoreDriver) Read(src string, data []byte) error {
	file, err := os.Open(v.updatePath(src))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Read(data)
	return err
}

func (v *VfsBlockStoreDriver) Write(data []byte, dst string) error {
	tmpFile := dst + ".tmp"
	if v.FileExists(tmpFile) {
		v.RemoveAll(tmpFile)
	}
	file, err := os.Create(v.updatePath(tmpFile))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)

	if v.FileExists(dst) {
		v.RemoveAll(dst)
	}
	return os.Rename(v.updatePath(tmpFile), v.updatePath(dst))
}

func (v *VfsBlockStoreDriver) List(path string) ([]string, error) {
	out, err := exec.Command("ls", "-1", v.updatePath(path)).Output()
	if err != nil {
		return nil, err
	}
	var result []string
	if len(out) == 0 {
		return result, nil
	}
	result = strings.Split(strings.TrimSpace(string(out)), "\n")
	return result, nil
}
