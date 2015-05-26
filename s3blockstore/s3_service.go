package s3blockstore

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/awserr"
	"github.com/awslabs/aws-sdk-go/aws/awsutil"
	"github.com/awslabs/aws-sdk-go/service/s3"
	"io"
	"os"
)

type S3Service struct {
	Keys   AwsKeys
	Region string
	Bucket string
}

type AwsKeys struct {
	AccessKey string
	SecretKey string
}

func (s *S3Service) New() (*s3.S3, error) {
	if err := os.Setenv(ENV_AWS_ACCESS_KEY, s.Keys.AccessKey); err != nil {
		return nil, err
	}
	if err := os.Setenv(ENV_AWS_SECRET_KEY, s.Keys.SecretKey); err != nil {
		return nil, err
	}
	return s3.New(&aws.Config{Region: s.Region}), nil
}

func (s *S3Service) Close() {
	if err := os.Setenv(ENV_AWS_ACCESS_KEY, ""); err != nil {
		log.Errorln("s3: Fail to cleanup S3 Access key, due to ", err)
	}
	if err := os.Setenv(ENV_AWS_SECRET_KEY, ""); err != nil {
		log.Errorln("s3: Fail to cleanup S3 Secret key, due to ", err)
	}
}

func parseAwsError(resp string, err error) error {
	log.Errorln(resp)
	if awsErr, ok := err.(awserr.Error); ok {
		message := fmt.Sprintln("AWS Error: ", awsErr.Code(), awsErr.Message(), awsErr.OrigErr())
		if reqErr, ok := err.(awserr.RequestFailure); ok {
			message += fmt.Sprintln(reqErr.StatusCode(), reqErr.RequestID())
		}
		return fmt.Errorf(message)
	}
	return err
}

func (s *S3Service) ListObjects(key string) ([]*s3.Object, error) {
	svc, err := s.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	// WARNING: Directory must end in "/" in S3, otherwise it may match
	// unintentially
	params := &s3.ListObjectsInput{
		Bucket: aws.String(s.Bucket),
		Prefix: aws.String(key),
	}
	resp, err := svc.ListObjects(params)
	if err != nil {
		return nil, parseAwsError(awsutil.StringValue(resp), err)
	}
	return resp.Contents, nil
}

func (s *S3Service) PutObject(key string, reader io.ReadSeeker) error {
	svc, err := s.New()
	if err != nil {
		return err
	}
	defer s.Close()

	params := &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
		Body:   reader,
	}

	resp, err := svc.PutObject(params)
	if err != nil {
		return parseAwsError(awsutil.StringValue(resp), err)
	}
	return nil
}

func (s *S3Service) GetObject(key string) (io.ReadCloser, error) {
	svc, err := s.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()

	params := &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}

	resp, err := svc.GetObject(params)
	if err != nil {
		return nil, parseAwsError(awsutil.StringValue(resp), err)
	}

	return resp.Body, nil
}

func (s *S3Service) DeleteObjects(key string) error {
	contents, err := s.ListObjects(key)
	if err != nil {
		return err
	}
	size := len(contents)
	if size == 0 {
		return nil
	}
	keyList := make([]string, size)
	for i, obj := range contents {
		keyList[i] = *obj.Key
	}

	svc, err := s.New()
	if err != nil {
		return err
	}
	defer s.Close()

	identifiers := make([]*s3.ObjectIdentifier, size)
	for i, k := range keyList {
		identifiers[i] = &s3.ObjectIdentifier{
			Key: aws.String(k),
		}
	}
	params := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.Bucket),
		Delete: &s3.Delete{
			Objects: identifiers,
			Quiet:   aws.Boolean(true),
		},
	}

	resp, err := svc.DeleteObjects(params)
	if err != nil {
		return parseAwsError(awsutil.StringValue(resp), err)
	}
	return nil
}
