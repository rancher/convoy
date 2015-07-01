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
	ENV_TEST_AWS_ACCESS_KEY = "RANCHER_TEST_AWS_ACCESS_KEY_ID"
	ENV_TEST_AWS_SECRET_KEY = "RANCHER_TEST_AWS_SECRET_ACCESS_KEY"
	ENV_TEST_AWS_REGION     = "RANCHER_TEST_AWS_REGION"
	ENV_TEST_AWS_BUCKET     = "RANCHER_TEST_AWS_BUCKET"
)

func (s *TestSuite) SetUpSuite(c *C) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(os.Stderr)

	s.service.Keys.AccessKey = os.Getenv(ENV_TEST_AWS_ACCESS_KEY)
	s.service.Keys.SecretKey = os.Getenv(ENV_TEST_AWS_SECRET_KEY)
	s.service.Region = os.Getenv(ENV_TEST_AWS_REGION)
	s.service.Bucket = os.Getenv(ENV_TEST_AWS_BUCKET)

	c.Assert(s.service.Keys.AccessKey, Not(Equals), "")
	c.Assert(s.service.Keys.SecretKey, Not(Equals), "")
	c.Assert(s.service.Region, Not(Equals), "")
	c.Assert(s.service.Bucket, Not(Equals), "")
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

	objs, err := s.service.ListObjects(key)
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 2)

	r, err := s.service.GetObject(key1)
	c.Assert(err, IsNil)

	newBody, err := ioutil.ReadAll(r)
	c.Assert(err, IsNil)
	c.Assert(newBody, DeepEquals, body)

	err = s.service.DeleteObjects([]string{key})
	c.Assert(err, IsNil)

	objs, err = s.service.ListObjects(key)
	c.Assert(err, IsNil)
	c.Assert(objs, HasLen, 0)
}
