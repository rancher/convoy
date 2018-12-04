package s3

import (
	"errors"
	"testing"

	"github.com/rancher/convoy/objectstore"
	"gopkg.in/check.v1"
)

var (
	errFakeConnection = errors.New("Simulated connection error")
)

func TestS3(t *testing.T) { check.TestingT(t) }

type S3TestSuite struct{}

var _ = check.Suite(&S3TestSuite{})

func runInitFunc(c *check.C, destURL, endpoint string, makeConnectionError bool) (bool, objectstore.ObjectStoreDriver, error) {
	attemptedConnection := false
	fakeConnectionTest := func(d objectstore.ObjectStoreDriver) error {
		attemptedConnection = true
		if makeConnectionError {
			return errFakeConnection
		}
		return nil
	}

	driver, err := initFuncWithConnectionCheck(destURL, endpoint, fakeConnectionTest)
	return attemptedConnection, driver, err
}

func (s *S3TestSuite) TestInitFuncBasicURL(c *check.C) {
	attemptedConnection, driver, err := runInitFunc(c, "s3://test@us-east-1/path", "", false)
	expectedDriver := S3ObjectStoreDriver{
		destURL: "s3://test@us-east-1/path",
		path:    "path",
		service: S3Service{
			Bucket: "test",
			Region: "us-east-1",
		},
	}

	c.Check(err, check.IsNil)
	c.Check(attemptedConnection, check.Equals, true)
	c.Check(&expectedDriver, check.DeepEquals, driver)
}

func (s *S3TestSuite) TestInitFuncNoPath(c *check.C) {
	attemptedConnection, driver, err := runInitFunc(c, "s3://test@us-east-1/", "", false)
	expectedDriver := S3ObjectStoreDriver{
		destURL: "s3://test@us-east-1/",
		service: S3Service{
			Bucket: "test",
			Region: "us-east-1",
		},
	}

	c.Check(err, check.IsNil)
	c.Check(attemptedConnection, check.Equals, true)
	c.Check(&expectedDriver, check.DeepEquals, driver)
}

func (s *S3TestSuite) TestInitFuncNoRegion(c *check.C) {
	attemptedConnection, driver, err := runInitFunc(c, "s3://test/", "", false)
	expectedDriver := S3ObjectStoreDriver{
		destURL: "s3://test/",
		service: S3Service{Bucket: "test"},
	}

	c.Check(err, check.IsNil)
	c.Check(attemptedConnection, check.Equals, true)
	c.Check(&expectedDriver, check.DeepEquals, driver)
}

func (s *S3TestSuite) TestInitFuncCustomEndpoint(c *check.C) {
	attemptedConnection, driver, err := runInitFunc(c, "s3://test@us-east-1/", "http://example.com", false)
	expectedDriver := S3ObjectStoreDriver{
		destURL: "s3://test@us-east-1/",
		service: S3Service{
			Bucket:   "test",
			Endpoint: "http://example.com",
			Region:   "us-east-1",
		},
	}

	c.Check(err, check.IsNil)
	c.Check(attemptedConnection, check.Equals, true)
	c.Check(&expectedDriver, check.DeepEquals, driver)
}

func (s *S3TestSuite) TestInitFuncBasicURLConnectionError(c *check.C) {
	_, _, err := runInitFunc(c, "s3://test@us-east-1/", "", true)
	c.Check(err, check.Equals, errFakeConnection)
}

func (s *S3TestSuite) TestInitFuncCustomEndpointConnectionError(c *check.C) {
	_, _, err := runInitFunc(c, "s3://test@us-east-1/", "http://example.com", true)
	c.Check(err, check.Equals, errFakeConnection)
}

func (s *S3TestSuite) TestInitFuncBadURL(c *check.C) {
	_, _, err := runInitFunc(c, ":", "", false)
	c.Check(err, check.NotNil)
}

func (s *S3TestSuite) TestInitFuncBadURLScheme(c *check.C) {
	_, _, err := runInitFunc(c, "http://test@us-east-1/", "", false)
	c.Check(err, check.NotNil)
}

func (s *S3TestSuite) TestInitFuncBadURLBucket(c *check.C) {
	_, _, err := runInitFunc(c, "s3://@us-east-1/", "", false)
	c.Check(err, check.NotNil)
}

func (s *S3TestSuite) TestInitFuncBadEndpointURL(c *check.C) {
	_, _, err := runInitFunc(c, "s3://test@us-east-1/", ":", false)
	c.Check(err, check.NotNil)
}

func (s *S3TestSuite) TestInitFuncTrimLeadingSlashes(c *check.C) {
	attemptedConnection, driver, err := runInitFunc(c, "s3://test@us-east-1////path", "", false)
	expectedDriver := S3ObjectStoreDriver{
		destURL: "s3://test@us-east-1/path",
		path:    "path",
		service: S3Service{
			Bucket: "test",
			Region: "us-east-1",
		},
	}

	c.Check(err, check.IsNil)
	c.Check(attemptedConnection, check.Equals, true)
	c.Check(&expectedDriver, check.DeepEquals, driver)
}
