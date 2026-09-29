package artifacts

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestClientUpload(t *testing.T) {
	testCases := []struct {
		name    string
		fail    int
		cancel  bool
		want    []string
		wantErr error
	}{
		{
			name: "uploads the directory",
			want: []string{"ns/name/uid/a.txt"},
		},
		{
			name:    "returns the write error",
			fail:    1,
			wantErr: errUpload,
		},
		{
			name:    "stops when the context is canceled",
			cancel:  true,
			wantErr: context.Canceled,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("log"), 0o644); err != nil {
				t.Fatal(err)
			}
			store := &fakeStore{fail: tc.fail}
			client := &Client{
				bucket: "bucket",
				prefix: "ns/name/uid",
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			err := client.uploadDir(ctx, dir, store.write)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Upload() error: %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Upload() error = %v, want %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, store.objects); diff != "" {
				t.Fatalf("Upload() objects mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

var errUpload = errors.New("bucket")

type fakeStore struct {
	fail    int
	objects []string
}

func (f *fakeStore) write(ctx context.Context, bucket, object string, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}
	if f.fail > 0 {
		f.fail--
		return errUpload
	}
	f.objects = append(f.objects, object)
	return nil
}
