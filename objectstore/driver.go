package objectstore

import (
	"fmt"
	"io"
	"net/url"

	"github.com/Sirupsen/logrus"

	. "github.com/rancher/convoy/logging"
)

type InitFunc func(destURL, endpoint string,accesskey string, secretkey string) (ObjectStoreDriver, error)

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

func GetObjectStoreDriver(destURL, endpoint string, accesskey string, secretkey string) (ObjectStoreDriver, error) {
	if destURL == "" {
		return nil, fmt.Errorf("Destination URL hasn't been specified")
	}
	u, destErr := url.Parse(destURL)
	if destErr != nil {
		return nil, destErr
	}
	if _, exists := initializers[u.Scheme]; !exists {
		return nil, fmt.Errorf("Driver %v is not supported!", u.Scheme)
	}
	if endpoint != "" {
		if u.Scheme != "s3" { // TODO change "s3" to use s3.KIND somehow? this causes import cycle at time of writing
			// only the S3 driver supports custom endpoints
			return nil, fmt.Errorf("Driver %v does not support custom endpoints", u.Scheme)
		}
		if _, err := url.Parse(endpoint); err != nil {
			return nil, err
		}
	}
	return initializers[u.Scheme](destURL, endpoint, accesskey, secretkey)
}
