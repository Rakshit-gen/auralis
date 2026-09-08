// Package objectstore wraps the MinIO client for S3-compatible storage
// (MinIO locally, Cloudflare R2 in production). It exposes presigned upload and
// download URLs so media never transits an application server.
package objectstore

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/auralis/platform/health"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config describes an S3-compatible endpoint.
type Config struct {
	Endpoint      string // host:port, no scheme
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	Bucket        string
	PublicBaseURL string // optional CDN/base URL for building stable object URLs
}

// Client is a thin wrapper over minio.Client bound to one bucket.
type Client struct {
	mc     *minio.Client
	bucket string
	public string
}

// New connects and ensures the bucket exists.
func New(ctx context.Context, cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, err
	}
	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, err
		}
	}
	return &Client{mc: mc, bucket: cfg.Bucket, public: cfg.PublicBaseURL}, nil
}

// PresignPut returns a URL the client can PUT an object to directly. The caller
// validates the object with Stat after the client reports completion.
func (c *Client) PresignPut(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := c.mc.PresignedPutObject(ctx, c.bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PresignGet returns a time-limited download URL for an object.
func (c *Client) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Stat returns the object size and content type, or an error if it is missing.
func (c *Client) Stat(ctx context.Context, key string) (size int64, contentType string, err error) {
	info, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", err
	}
	return info.Size, info.ContentType, nil
}

// PutBytes uploads a small object directly (used by workers, not request paths).
func (c *Client) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// PutReader streams an object of known size.
func (c *Client) PutReader(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size,
		minio.PutObjectOptions{ContentType: contentType})
	return err
}

// GetBytes downloads a small object fully into memory.
func (c *Client) GetBytes(ctx context.Context, key string) ([]byte, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(obj)
}

// Remove deletes an object.
func (c *Client) Remove(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}

// PublicURL returns a stable public URL for a key when a base URL is configured.
func (c *Client) PublicURL(key string) string {
	if c.public == "" {
		return ""
	}
	return c.public + "/" + key
}

// ReadyCheck reports whether the bucket is reachable.
func (c *Client) ReadyCheck() health.Check {
	return func(ctx context.Context) error {
		_, err := c.mc.BucketExists(ctx, c.bucket)
		return err
	}
}
