package s3

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/rancher/rancher-volume/objectstore"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "s3"})
)

type S3ObjectStoreDriver struct {
	Path    string
	Service S3Service
}

const (
	KIND = "s3"
)

func init() {
	objectstore.RegisterDriver(KIND, initFunc)
}

func initFunc(destURL string) (objectstore.ObjectStoreDriver, error) {
	b := &S3ObjectStoreDriver{}

	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}

	if u.Scheme != KIND {
		return nil, fmt.Errorf("BUG: Why dispatch %v to %v?", u.Scheme, KIND)
	}

	if u.User != nil {
		b.Service.Region = u.Host
		b.Service.Bucket = u.User.Username()
	} else {
		//We would depends on AWS_REGION environment variable
		b.Service.Bucket = u.Host
	}
	b.Path = u.Path
	if b.Service.Bucket == "" || b.Path == "" {
		return nil, fmt.Errorf("Invalid URL. Must be either s3://bucket@region/path/, or s3://bucket/path")
	}

	//Leading '/' can cause mystery problems for s3
	b.Path = strings.TrimLeft(b.Path, "/")

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
