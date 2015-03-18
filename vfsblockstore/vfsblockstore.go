package vfsblockstore

import (
	"fmt"
	"github.com/yasker/volmgr/blockstores"
	"github.com/yasker/volmgr/utils"
	"os"
	"os/exec"
	"path/filepath"
)

type VfsBlockStore struct {
	Id   string
	Path string
}

const (
	KIND = "vfs"

	VFS_PATH = "vfs.path"
)

func init() {
	blockstores.Register(KIND, initFunc)
}

func initFunc(configFile, id string, config map[string]string) (blockstores.BlockStore, error) {
	b := &VfsBlockStore{}
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

func (v *VfsBlockStore) Kind() string {
	return KIND
}

func (v *VfsBlockStore) FileExists(fileName string) bool {
	file := filepath.Join(v.Path, fileName)
	st, err := os.Stat(file)
	return err == nil && !st.IsDir()
}

func (v *VfsBlockStore) MkDir(dirName string) error {
	return os.MkdirAll(dirName, os.ModeDir|0700)
}

func (v *VfsBlockStore) Read(srcPath, srcFileName string, data []byte) error {
	file, err := os.Open(filepath.Join(srcPath, srcFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Read(data)
	return err
}

func (v *VfsBlockStore) Write(data []byte, dstPath, dstFileName string) error {
	file, err := os.Create(filepath.Join(dstPath, dstFileName))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	return err
}

func (v *VfsBlockStore) CopyToPath(srcFileName string, path string) error {
	err := exec.Command("cp", srcFileName, path).Run()
	return err
}
