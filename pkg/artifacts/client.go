package artifacts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

const uploadAttempts = 3

// Client uploads a directory to one GCS prefix.
type Client struct {
	storage *storage.Client
	bucket  string
	prefix  string
}

// NewClient authenticates with the service account file and uploads under namespace/name/uid.
func NewClient(ctx context.Context, credentialsFile, bucket, namespace, name, uid string) (*Client, error) {
	sc, err := storage.NewClient(ctx, option.WithAuthCredentialsFile(option.ServiceAccount, credentialsFile))
	if err != nil {
		return nil, err
	}
	sc.SetRetry(storage.WithMaxAttempts(uploadAttempts), storage.WithPolicy(storage.RetryAlways))
	return &Client{
		storage: sc,
		bucket:  bucket,
		prefix:  path.Join(namespace, name, uid),
	}, nil
}

// Close closes the GCS client.
func (c *Client) Close() error {
	return c.storage.Close()
}

// Upload stores every regular file under dir.
func (c *Client) Upload(ctx context.Context, dir string) error {
	if err := c.uploadDir(ctx, dir, c.write); err != nil {
		return err
	}
	logrus.Info("artifact upload finished")
	return nil
}

func (c *Client) uploadDir(ctx context.Context, dir string, write func(context.Context, string, string, io.Reader) error) error {
	files, err := ListFiles(dir)
	if err != nil {
		return err
	}
	var uploadErr error
	for _, rel := range files {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			uploadErr = errors.Join(uploadErr, fmt.Errorf("open %s: %w", rel, err))
			continue
		}
		object := path.Join(c.prefix, rel)
		err = write(ctx, c.bucket, object, f)
		f.Close()
		if err != nil {
			uploadErr = errors.Join(uploadErr, fmt.Errorf("upload %s: %w", object, err))
		}
	}
	return uploadErr
}

func (c *Client) write(ctx context.Context, bucket, object string, r io.Reader) error {
	w := c.storage.Bucket(bucket).Object(object).NewWriter(ctx)
	if _, err := io.Copy(w, r); err != nil {
		return errors.Join(err, w.Close())
	}
	return w.Close()
}
