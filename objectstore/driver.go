package objectstore

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"io"
	"net/url"

	. "github.com/rancher/rancher-volume/logging"
)

type InitFunc func(destURL string) (ObjectStoreDriver, error)

type ObjectStoreDriver interface {
	Kind() string
	GetURL() string
	FileExists(filePath string) bool
	FileSize(filePath string) int64
	Remove(names ...string) error           // Bahavior like "rm -rf"
	Read(src string) (io.ReadCloser, error) // Caller needs to close
	Write(dst string, rs io.ReadSeeker) error
	List(path string) ([]string, error) // Behavior like "ls", not like "find"
	Upload(src, dst string) error
	Download(src, dst string) error
}

var (
	initializers map[string]InitFunc
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "objectstore"})
)

func generateError(fields logrus.Fields, format string, v ...interface{}) error {
	return ErrorWithFields("objectstore", fields, format, v)
}

func init() {
	initializers = make(map[string]InitFunc)
}

func RegisterDriver(kind string, initFunc InitFunc) error {
	if _, exists := initializers[kind]; exists {
		return fmt.Errorf("%s has already been registered", kind)
	}
	initializers[kind] = initFunc
	return nil
}

func getObjectStoreDriver(destURL string) (ObjectStoreDriver, error) {
	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}
	if _, exists := initializers[u.Scheme]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", u.Scheme)
	}
	return initializers[u.Scheme](destURL)
}
