package blockstores

import (
	"fmt"
)

type InitFunc func(configFile, id string, config map[string]string) (BlockStore, error)

type BlockStore interface {
	Kind() string
	FileExists(fileName string) bool
	MkDir(dirName string) error
	Read(srcPath, srcFileName string, data []byte) error
	Write(data []byte, dstPath, dstFileName string) error
	CopyToPath(srcFileName string, path string) error
}

var (
	initializers map[string]InitFunc
)

func init() {
	initializers = make(map[string]InitFunc)
}

func Register(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func GetBlockStore(kind, configFile, id string, config map[string]string) (BlockStore, error) {
	if _, exists := initializers[kind]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", kind)
	}
	return initializers[kind](configFile, id, config)
}
