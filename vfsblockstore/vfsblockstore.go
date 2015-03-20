package vfsblockstore

import (
	"fmt"
	"github.com/yasker/volmgr/blockstores"
	"github.com/yasker/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
)

type VfsBlockStoreDriver struct {
	Id   string
	Path string
}

const (
	KIND = "vfs"

	VFS_PATH = "vfs.path"
)

func init() {
	blockstores.RegisterDriver(KIND, initFunc)
}

func initFunc(configFile, id string, config map[string]string) (blockstores.BlockStoreDriver, error) {
	b := &VfsBlockStoreDriver{}
	if _, err := os.Stat(configFile); err == nil {
		err := utils.LoadConfig(configFile, b)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	b.Id = id
	b.Path = config[VFS_PATH]
	if b.Path == "" {
		return nil, fmt.Errorf("Cannot find required field %v", VFS_PATH)
	}
	if st, err := os.Stat(b.Path); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("VFS path %v doesn't exist or is not a directory", b.Path)
	}
	err := utils.SaveConfig(configFile, b)
	if err != nil {
		return nil, err
	}

	return b, nil
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
