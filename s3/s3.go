package s3

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/rancher/convoy/objectstore"
)

var (
	log = logrus.WithFields(logrus.Fields{"pkg": "s3"})
)

type S3ObjectStoreDriver struct {
	destURL string
	path    string
	service S3Service
}

const (
	KIND = "s3"
)

func init() {
	if err := objectstore.RegisterDriver(KIND, initFunc); err != nil {
		panic(err)
	}
}

func initFunc(destURL, endpoint string, accesskey string, secretkey string) (objectstore.ObjectStoreDriver, error) {
	return initFuncWithConnectionCheck(destURL, endpoint, accesskey, secretkey, func(d objectstore.ObjectStoreDriver) error {
		_, err := d.List("")
		return err
	})
}

func initFuncWithConnectionCheck(destURL, endpoint string, accesskey string, secretkey string, connectionTest func(d objectstore.ObjectStoreDriver) error) (objectstore.ObjectStoreDriver, error) {
	b := &S3ObjectStoreDriver{}

	u, err := url.Parse(destURL)
	if err != nil {
		return nil, err
	}

	if _, err := url.Parse(endpoint); err != nil {
		return nil, err
	}
	b.service.Endpoint = endpoint
	b.service.Accesskey = accesskey
	b.service.Secretkey = secretkey

	if u.Scheme != KIND {
		return nil, fmt.Errorf("BUG: Why dispatch %v to %v?", u.Scheme, KIND)
	}

	if u.User != nil {
		b.service.Region = u.Host
		b.service.Bucket = u.User.Username()
	} else {
		//We would depends on AWS_REGION environment variable
		b.service.Bucket = u.Host
	}
	b.path = u.Path
	if b.service.Bucket == "" || b.path == "" {
		return nil, fmt.Errorf("Invalid URL. Must be either s3://bucket@region/path/, or s3://bucket/path")
	}

	//Leading '/' can cause mystery problems for s3
	b.path = strings.TrimLeft(b.path, "/")

	//Test connection
	if err := connectionTest(b); err != nil {
		return nil, err
	}

	b.destURL = KIND + "://" + b.service.Bucket
	if b.service.Region != "" {
		b.destURL += "@" + b.service.Region
	}
	b.destURL += "/" + b.path

	log.Debug("Loaded driver for %v", b.destURL)
	return b, nil
}

func (s *S3ObjectStoreDriver) Kind() string {
	return KIND
}

func (s *S3ObjectStoreDriver) GetURL() string {
	return s.destURL
}

func (s *S3ObjectStoreDriver) updatePath(path string) string {
	return filepath.Join(s.path, path)
}

func (s *S3ObjectStoreDriver) List(listPath string) ([]string, error) {
	var result []string

	path := s.updatePath(listPath) + "/"
	contents, prefixes, err := s.service.ListObjects(path, "/")
	if err != nil {
		log.Error("Fail to list s3: ", err)
		return result, err
	}

	sizeC := len(contents)
	sizeP := len(prefixes)
	if sizeC == 0 && sizeP == 0 {
		return result, nil
	}
	result = []string{}
	for _, obj := range contents {
		r := strings.TrimPrefix(*obj.Key, path)
		if r != "" {
			result = append(result, r)
		}
	}
	for _, p := range prefixes {
		r := strings.TrimPrefix(*p.Prefix, path)
		r = strings.TrimSuffix(r, "/")
		if r != "" {
			result = append(result, r)
		}
	}

	return result, nil
}

func (s *S3ObjectStoreDriver) FileExists(filePath string) bool {
	return s.FileSize(filePath) >= 0
}

func (s *S3ObjectStoreDriver) FileSize(filePath string) int64 {
	path := s.updatePath(filePath)
	head, err := s.service.HeadObject(path)
	if err != nil {
		return -1
	}
	if head.ContentLength == nil {
		return -1
	}
	return *head.ContentLength
}

func (s *S3ObjectStoreDriver) Remove(names ...string) error {
	if len(names) == 0 {
		return nil
	}
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = s.updatePath(name)
	}
	return s.service.DeleteObjects(paths)
}

func (s *S3ObjectStoreDriver) Read(src string) (io.ReadCloser, error) {
	path := s.updatePath(src)
	rc, err := s.service.GetObject(path)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func (s *S3ObjectStoreDriver) Write(dst string, rs io.ReadSeeker) error {
	path := s.updatePath(dst)
	return s.service.PutObject(path, rs)
}

func (s *S3ObjectStoreDriver) Upload(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return nil
	}
	defer file.Close()
	path := s.updatePath(dst)
	return s.service.PutObject(path, file)
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
	rc, err := s.service.GetObject(path)
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
