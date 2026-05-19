package storage

import (
	"context"
	"fmt"
	"io"
	"os"
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

	if _, ok := reader.(io.ReadSeeker); !ok {
		tmpFile, err := os.CreateTemp("", "moonlight-s3-*")
		if err != nil {
			return err
		}
		defer tmpFile.Close()
		defer os.Remove(tmpFile.Name())

		if _, err := io.Copy(tmpFile, reader); err != nil {
			return err
		}
		if _, err := tmpFile.Seek(0, 0); err != nil {
			return err
		}
		reader = tmpFile
	}

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(fullKey),
		Body:          reader,
		ContentLength: aws.Int64(size),
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
		return 0, fmt.Errorf("S3 HeadObject returned nil ContentLength for key: %s", fullKey)
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

func (s *S3Storage) BasePath() string {
	return s.basePath
}

func (s *S3Storage) Browse(ctx context.Context, path string) ([]BrowseEntry, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	cleanPath = strings.TrimSuffix(cleanPath, "/")

	var prefix string
	if cleanPath == "" {
		prefix = s.basePath
	} else {
		if s.basePath != "" {
			prefix = s.basePath + "/" + cleanPath
		} else {
			prefix = cleanPath
		}
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	result := make([]BrowseEntry, 0, len(resp.CommonPrefixes)+len(resp.Contents))

	for _, cp := range resp.CommonPrefixes {
		p := *cp.Prefix
		var name string
		if s.basePath != "" {
			name = strings.TrimPrefix(p, s.basePath+"/")
		} else {
			name = p
		}
		name = strings.TrimSuffix(name, "/")

		var entryPath string
		if cleanPath == "" {
			entryPath = name
		} else {
			entryPath = cleanPath + "/" + name
		}

		result = append(result, BrowseEntry{
			Name:    name,
			Path:    entryPath,
			IsDir:   true,
			Size:    0,
			ModTime: "-",
		})
	}

	for _, obj := range resp.Contents {
		key := *obj.Key
		if key == prefix {
			continue
		}

		var name string
		if s.basePath != "" {
			name = strings.TrimPrefix(key, s.basePath+"/")
		} else {
			name = key
		}
		name = strings.TrimPrefix(name, cleanPath+"/")

		var entryPath string
		if cleanPath == "" {
			entryPath = name
		} else {
			entryPath = cleanPath + "/" + name
		}

		modTime := "-"
		if obj.LastModified != nil {
			modTime = obj.LastModified.Format("2006-01-02 15:04:05")
		}

		var size int64
		if obj.Size != nil {
			size = *obj.Size
		}

		result = append(result, BrowseEntry{
			Name:    name,
			Path:    entryPath,
			IsDir:   false,
			Size:    size,
			ModTime: modTime,
		})
	}

	return result, nil
}
