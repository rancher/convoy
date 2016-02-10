// +build s3test

package s3

import (
	"bytes"
	"github.com/Sirupsen/logrus"
	"io/ioutil"
	"os"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	service S3Service
}

var _ = Suite(&TestSuite{})

const (
	ENV_TEST_AWS_REGION = "CONVOY_TEST_AWS_REGION"
	ENV_TEST_AWS_BUCKET = "CONVOY_TEST_AWS_BUCKET"
)

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)

	s.service.Region = os.Getenv(ENV_TEST_AWS_REGION)
	s.service.Bucket = os.Getenv(ENV_TEST_AWS_BUCKET)

	if s.service.Region == "" || s.service.Bucket == "" {
		c.Skip("S3 test environment variables not provided.")
	}
}

func (s *TestSuite) TestFuncs(c *C) {
	var err error
	body := []byte("this is only a test file")

	key := "test_file"
	key1 := "test_file_1"
	key2 := "test_file_2"

	err = s.service.PutObject(key1, bytes.NewReader(body))
	c.Assert(err, IsNil)
	err = s.service.PutObject(key2, bytes.NewReader(body))
	c.Assert(err, IsNil)

	objs, _, err := s.service.ListObjects(key, "")
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 2)

	r, err := s.service.GetObject(key1)
	c.Assert(err, IsNil)

	newBody, err := ioutil.ReadAll(r)
	c.Assert(err, IsNil)
	c.Assert(newBody, DeepEquals, body)

	err = s.service.DeleteObjects([]string{key})
	c.Assert(err, IsNil)

	objs, _, err = s.service.ListObjects(key, "")
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 0)
}

func (s *TestSuite) TestList(c *C) {
	var err error

	body := []byte("this is only a test file")
	dir1_key1 := "dir/dir1/test_file_1"
	dir1_key2 := "dir/dir1/test_file_2"
	dir2_key1 := "dir/dir2/test_file_1"
	dir2_key2 := "dir/dir2/test_file_2"

	err = s.service.PutObject(dir1_key1, bytes.NewReader(body))
	c.Assert(err, IsNil)
	err = s.service.PutObject(dir1_key2, bytes.NewReader(body))
	c.Assert(err, IsNil)
	err = s.service.PutObject(dir2_key1, bytes.NewReader(body))
	c.Assert(err, IsNil)
	err = s.service.PutObject(dir2_key2, bytes.NewReader(body))
	c.Assert(err, IsNil)

	objs, prefixes, err := s.service.ListObjects("dir/", "/")
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 0)
	c.Assert(prefixes, HasLen, 2)

	objs, prefixes, err = s.service.ListObjects("dir/dir1/", "/")
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 2)
	c.Assert(prefixes, HasLen, 0)

	err = s.service.DeleteObjects([]string{"dir"})
	c.Assert(err, IsNil)

	objs, prefixes, err = s.service.ListObjects("dir/", "")
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 0)
	c.Assert(prefixes, HasLen, 0)
}
