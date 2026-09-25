package backupapi

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3CompatibleStore is a project-scoped S3-compatible client. It accepts only
// an HTTPS origin (no URL credentials, path, query, or fragment) and always
// uses path-style bucket lookup for compatibility with private object stores.
type S3CompatibleStore struct {
	client *minio.Client
	bucket string
}

func NewS3CompatibleStore(endpoint, bucket, accessKey, secretKey, region string) (*S3CompatibleStore, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid S3 endpoint")
	}
	if bucket == "" || accessKey == "" || secretKey == "" {
		return nil, errors.New("incomplete S3 configuration")
	}
	client, err := minio.New(parsed.Host, &minio.Options{
		Creds:        credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:       true,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, errors.New("invalid S3 configuration")
	}
	return &S3CompatibleStore{client: client, bucket: bucket}, nil
}

func (s *S3CompatibleStore) Put(ctx context.Context, key string, body io.Reader, size int64) error {
	if s == nil || s.client == nil || !validObjectKey(key) || size <= 0 {
		return errors.New("invalid object upload")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return errors.New("object upload failed")
	}
	return nil
}

func (s *S3CompatibleStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if s == nil || s.client == nil || !validObjectKey(key) {
		return nil, errors.New("invalid object read")
	}
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.New("object read failed")
	}
	// GetObject returns errors lazily from Read; verify existence and response
	// metadata here so 404s are reported before returning the stream.
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		return nil, errors.New("object read failed")
	}
	return object, nil
}

func validObjectKey(key string) bool {
	if len(key) != 36 || strings.ContainsAny(key, "/?#") {
		return false
	}
	_, err := uuid.Parse(key)
	return err == nil
}
