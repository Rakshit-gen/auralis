package objectstore

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestPresignPutSignsTypeAndSize(t *testing.T) {
	// Presigning is local when the region is known, so no server is needed.
	mc, err := minio.New("localhost:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("key", "secret", ""),
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{mc: mc, bucket: "media"}
	raw, err := c.PresignPut(context.Background(), "source/a.mp3", time.Minute, "audio/mpeg", 4096)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	signed := u.Query().Get("X-Amz-SignedHeaders")
	for _, h := range []string{"content-length", "content-type"} {
		if !strings.Contains(signed, h) {
			t.Fatalf("%s is not signed: %q", h, signed)
		}
	}
}
