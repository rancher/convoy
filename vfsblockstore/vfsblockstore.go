package vfsblockstore

import (
	"fmt"
	"github.com/rancherio/volmgr/blockstores"
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
	blockstores.RegisterDriver(KIND, initFunc)
}

func initFunc(root, cfgName string, config map[string]string) (blockstores.BlockStoreDriver, error) {
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

func (v *VfsBlockStoreDriver) FileSize(path, fileName string) int64 {
	file := filepath.Join(v.Path, path, fileName)
	st, err := os.Stat(file)
	if err != nil || st.IsDir() {
		return -1
	}
	return st.Size()
}

func (v *VfsBlockStoreDriver) FileExists(path, fileName string) bool {
	return v.FileSize(path, fileName) >= 0
}

func (v *VfsBlockStoreDriver) MkDirAll(dirName string) error {
	return os.MkdirAll(filepath.Join(v.Path, dirName), os.ModeDir|0700)
}

func (v *VfsBlockStoreDriver) RemoveAll(name string) error {
	return os.RemoveAll(filepath.Join(v.Path, name))
}

func (v *VfsBlockStoreDriver) Read(srcPath, srcFileName string, data []byte) error {
	file, err := os.Open(filepath.Join(v.Path, srcPath, srcFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Read(data)
	return err
}

func (v *VfsBlockStoreDriver) Write(data []byte, dstPath, dstFileName string) error {
	file, err := os.Create(filepath.Join(v.Path, dstPath, dstFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (v *VfsBlockStoreDriver) CopyToPath(srcFileName string, path string) error {
	err := exec.Command("cp", filepath.Join(v.Path, srcFileName), path).Run()
	return err
}

func (v *VfsBlockStoreDriver) List(path string) ([]string, error) {
	out, err := exec.Command("ls", "-1", filepath.Join(v.Path, path)).Output()
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

func (v *VfsBlockStoreDriver) Rename(srcName, dstName string) error {
	return exec.Command("mv", "-f",
		filepath.Join(v.Path, srcName),
		filepath.Join(v.Path, dstName)).Run()
}
