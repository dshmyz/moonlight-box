package storage

import (
	"context"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	client   *s3.Client
	bucket   string
	basePath string
	maxSize  int64
}

func NewS3Storage(endpoint, region, accessKeyID, secretAccessKey, bucket, basePath string, maxSizeGB int64, useSSL bool) (*S3Storage, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}

	opts := []func(*s3.Options){
		func(o *s3.Options) {
			o.UsePathStyle = true
		},
	}

	if endpoint != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	client := s3.NewFromConfig(cfg, opts...)

	return &S3Storage{
		client:   client,
		bucket:   bucket,
		basePath: strings.TrimPrefix(basePath, "/"),
		maxSize:  maxSizeGB * 1024 * 1024 * 1024,
	}, nil
}

func (s *S3Storage) Name() string {
	return "s3"
}

func (s *S3Storage) Init(_ string) error {
	return nil
}

func (s *S3Storage) resolveKey(key string) string {
	key = strings.TrimPrefix(key, "/")
	if s.basePath != "" {
		return s.basePath + "/" + key
	}
	return key
}

func (s *S3Storage) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	fullKey := s.resolveKey(key)

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
		Body:   reader,
	})
	return err
}

func (s *S3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullKey := s.resolveKey(key)

	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	fullKey := s.resolveKey(key)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	return err
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := s.resolveKey(key)

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Storage) Size(ctx context.Context, key string) (int64, error) {
	fullKey := s.resolveKey(key)

	resp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		return 0, err
	}

	if resp.ContentLength == nil {
		return 0, nil
	}
	return *resp.ContentLength, nil
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]Entry, error) {
	fullPrefix := s.resolveKey(prefix)

	resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(fullPrefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	result := make([]Entry, 0, len(resp.Contents)+len(resp.CommonPrefixes))

	for _, obj := range resp.Contents {
		key := strings.TrimPrefix(*obj.Key, s.basePath+"/")
		result = append(result, Entry{
			Key:   key,
			IsDir: false,
			Size:  *obj.Size,
		})
	}

	for _, cp := range resp.CommonPrefixes {
		key := strings.TrimPrefix(*cp.Prefix, s.basePath+"/")
		result = append(result, Entry{
			Key:   key,
			IsDir: true,
		})
	}

	return result, nil
}

func (s *S3Storage) Close() error {
	return nil
}
