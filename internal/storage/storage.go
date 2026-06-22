package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/felipedsvit/erreia/internal/config"
)

// ObjectStore is the storage port used by the rest of the app.
// The MinIO implementation is the only one we ship today; this interface
// keeps it easy to swap for S3, GCS, or an in-memory fake in tests.
type ObjectStore interface {
	Put(ctx context.Context, key, contentType string, r io.Reader) error
	Delete(ctx context.Context, key string) error
	PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Get(ctx context.Context, key string) (io.ReadCloser, string, error)
}

type MinIOStore struct {
	client        *minio.Client
	bucket        string
	presignClient *minio.Client
	presignTTL    time.Duration
}

func NewMinIO(cfg *config.Config) (*MinIOStore, error) {
	internal, err := minio.New(cfg.StorageEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.StorageAccessKey, cfg.StorageSecretKey, ""),
		Secure: cfg.StorageUseSSL,
		Region: cfg.StorageRegion,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client (internal): %w", err)
	}

	publicHost, useSSL := splitEndpoint(cfg.StoragePublicEndpoint)
	presign, err := minio.New(publicHost, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.StorageAccessKey, cfg.StorageSecretKey, ""),
		Secure: useSSL,
		Region: cfg.StorageRegion,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client (public): %w", err)
	}

	return &MinIOStore{
		client:        internal,
		bucket:        cfg.StorageBucket,
		presignClient: presign,
		presignTTL:    cfg.StoragePresignTTL,
	}, nil
}

// splitEndpoint accepts "host:port", "http://host:port", or "https://host:port"
// and returns (host:port, useSSL). The minio-go client requires a bare host:port
// for the Endpoint field; the scheme is communicated via the Secure option.
func splitEndpoint(raw string) (string, bool) {
	useSSL := false
	switch {
	case len(raw) >= 8 && raw[:8] == "https://":
		useSSL = true
		raw = raw[8:]
	case len(raw) >= 7 && raw[:7] == "http://":
		raw = raw[7:]
	}
	return raw, useSSL
}

// Put uploads an object using the internal client.
func (m *MinIOStore) Put(ctx context.Context, key, contentType string, r io.Reader) error {
	_, err := m.client.PutObject(ctx, m.bucket, key, r, -1, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// Delete removes an object. Missing keys are not an error.
func (m *MinIOStore) Delete(ctx context.Context, key string) error {
	_ = m.client.RemoveObject(ctx, m.bucket, key, minio.RemoveObjectOptions{})
	return nil
}

// PresignedGet returns a short-lived signed URL for a browser fetch.
func (m *MinIOStore) PresignedGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = m.presignTTL
	}
	u, err := m.presignClient.PresignedGetObject(ctx, m.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Get streams the object from the internal client. Used by the avatar
// proxy handler so the browser can fetch avatars from the same origin
// (avoids opening img-src to the storage host in the CSP).
func (m *MinIOStore) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", err
	}
	stat, err := obj.Stat()
	if err != nil {
		_ = obj.Close()
		return nil, "", err
	}
	ct := stat.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	return obj, ct, nil
}
