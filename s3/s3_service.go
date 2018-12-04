package s3

import (
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3Service struct {
	Region   string
	Bucket   string
	Endpoint string
}

func (s *S3Service) New() (*s3.S3, error) {
	config := aws.NewConfig().
		WithRegion(s.Region)

	if s.Endpoint != "" {
		config = config.
			WithEndpoint(s.Endpoint).
			WithS3ForcePathStyle(true)
	}
	return s3.New(session.New(), config), nil
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

func (s *S3Service) ListObjects(key, delimiter string) ([]*s3.Object, []*s3.CommonPrefix, error) {
	svc, err := s.New()
	if err != nil {
		return nil, nil, err
	}
	defer s.Close()
	// WARNING: Directory must end in "/" in S3, otherwise it may match
	// unintentially
	params := &s3.ListObjectsInput{
		Bucket:    aws.String(s.Bucket),
		Prefix:    aws.String(key),
		Delimiter: aws.String(delimiter),
	}
	resp, err := svc.ListObjects(params)
	if err != nil {
		return nil, nil, parseAwsError(resp.String(), err)
	}
	return resp.Contents, resp.CommonPrefixes, nil
}

func (s *S3Service) HeadObject(key string) (*s3.HeadObjectOutput, error) {
	svc, err := s.New()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	params := &s3.HeadObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.HeadObject(params)
	if err != nil {
		return nil, parseAwsError(resp.String(), err)
	}
	return resp, nil
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
		return parseAwsError(resp.String(), err)
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
		return nil, parseAwsError(resp.String(), err)
	}

	return resp.Body, nil
}

func (s *S3Service) DeleteObjects(keys []string) error {
	var keyList []string
	totalSize := 0
	for _, key := range keys {
		contents, _, err := s.ListObjects(key, "")
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
	quiet := true
	params := &s3.DeleteObjectsInput{
		Bucket: aws.String(s.Bucket),
		Delete: &s3.Delete{
			Objects: identifiers,
			Quiet:   &quiet,
		},
	}

	resp, err := svc.DeleteObjects(params)
	if err != nil {
		return parseAwsError(resp.String(), err)
	}
	return nil
}
