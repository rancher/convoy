package s3

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/service/s3"
	"io"
)

type S3Service struct {
	Region string
	Bucket string
}

func (s *S3Service) New() (*s3.S3, error) {
	return s3.New(&aws.Config{Region: s.Region}), nil
}

func (s *S3Service) Close() {
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

func (s *S3Service) DeleteObjects(keys []string) error {
	var keyList []string
	totalSize := 0
	for _, key := range keys {
		contents, err := s.ListObjects(key)
		if err != nil {
			return err
		}
		size := len(contents)
		if size == 0 {
			continue
		}
		totalSize += size
		for _, obj := range contents {
			keyList = append(keyList, *obj.Key)
		}
	}

	svc, err := s.New()
	if err != nil {
		return err
	}
	defer s.Close()

	identifiers := make([]*s3.ObjectIdentifier, totalSize)
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
