package s3

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/objectstore"
	"github.com/rancher/rancher-volume/util"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "s3"})
)

type S3ObjectStoreDriver struct {
	ID      string
	Path    string
	Service S3Service
}

const (
	KIND = "s3"

	S3_REGION = "s3.region"
	S3_BUCKET = "s3.bucket"
	S3_PATH   = "s3.path"
)

func init() {
	objectstore.RegisterDriver(KIND, initFunc)
}

func initFunc(root, cfgName string, config map[string]string) (objectstore.ObjectStoreDriver, error) {
	b := &S3ObjectStoreDriver{}
	if cfgName != "" {
		if util.ConfigExists(root, cfgName) {
			err := util.LoadConfig(root, cfgName, b)
			if err != nil {
				return nil, err
			}
			return b, nil
		} else {
			return nil, fmt.Errorf("Wrong configuration file for S3 objectstore driver")
		}
	}

	b.Service.Region = config[S3_REGION]
	b.Service.Bucket = config[S3_BUCKET]
	b.Path = config[S3_PATH]
	if b.Service.Region == "" || b.Service.Bucket == "" || b.Path == "" {
		return nil, fmt.Errorf("Cannot find all required fields: %v %v %v",
			S3_REGION, S3_BUCKET, S3_PATH)
	}

	if strings.HasPrefix(b.Path, "/") {
		return nil, fmt.Errorf("Slash '/' is not allowed at beginning of path: %v", b.Path)
	}

	//Test connection
	if _, err := b.List(""); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *S3ObjectStoreDriver) Kind() string {
	return KIND
}

func (s *S3ObjectStoreDriver) updatePath(path string) string {
	return filepath.Join(s.Path, path)
}

func (s *S3ObjectStoreDriver) FinalizeInit(root, cfgName, id string) error {
	s.ID = id
	if err := util.SaveConfig(root, cfgName, s); err != nil {
		return err
	}
	return nil
}

func (s *S3ObjectStoreDriver) List(listPath string) ([]string, error) {
	var result []string

	path := s.updatePath(listPath)
	contents, err := s.Service.ListObjects(path)
	if err != nil {
		log.Error("Fail to list s3: ", err)
		return result, err
	}

	size := len(contents)
	if size == 0 {
		return result, nil
	}
	result = make([]string, size)
	for i, obj := range contents {
		result[i] = strings.TrimPrefix(*obj.Key, path)
	}

	return result, nil
}

func (s *S3ObjectStoreDriver) FileExists(filePath string) bool {
	return s.FileSize(filePath) >= 0
}

func (s *S3ObjectStoreDriver) FileSize(filePath string) int64 {
	path := s.updatePath(filePath)
	contents, err := s.Service.ListObjects(path)
	if err != nil {
		return -1
	}

	if len(contents) == 0 {
		return -1
	}

	//TODO deal with multiple returns
	return *contents[0].Size
}

func (s *S3ObjectStoreDriver) Remove(names ...string) error {
	if len(names) == 0 {
		return nil
	}
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = s.updatePath(name)
	}
	return s.Service.DeleteObjects(paths)
}

func (s *S3ObjectStoreDriver) Read(src string) (io.ReadCloser, error) {
	path := s.updatePath(src)
	rc, err := s.Service.GetObject(path)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (s *S3ObjectStoreDriver) Write(dst string, rs io.ReadSeeker) error {
	path := s.updatePath(dst)
	return s.Service.PutObject(path, rs)
}

func (s *S3ObjectStoreDriver) Upload(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return nil
	}
	defer file.Close()
	path := s.updatePath(dst)
	return s.Service.PutObject(path, file)
}

func (s *S3ObjectStoreDriver) Download(src, dst string) error {
	if _, err := os.Stat(dst); err != nil {
		os.Remove(dst)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	path := s.updatePath(src)
	rc, err := s.Service.GetObject(path)
	if err != nil {
		return err
	}
	defer rc.Close()

	_, err = io.Copy(f, rc)
	if err != nil {
		return err
	}
	return nil
}
