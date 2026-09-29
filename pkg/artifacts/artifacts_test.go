package artifacts

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListFiles(t *testing.T) {
	testCases := []struct {
		name  string
		dir   func(tmp string) string
		setup func(t *testing.T, dir string)
		want  []string
	}{
		{
			name: "nested file",
			setup: func(t *testing.T, dir string) {
				writeArtifact(t, dir, "b.txt", "top")
				writeArtifact(t, dir, "sub/a.txt", "nested")
			},
			want: []string{"b.txt", "sub/a.txt"},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, dir string) {
				writeArtifact(t, dir, "a.txt", "one")
				if err := os.Symlink("a.txt", filepath.Join(dir, "a.link")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"a.txt"},
		},
		{
			name: "symlink directory",
			setup: func(t *testing.T, dir string) {
				writeArtifact(t, dir, "real/file.txt", "one")
				if err := os.Symlink("real", filepath.Join(dir, "link")); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"real/file.txt"},
		},
		{
			name: "missing directory",
			dir: func(tmp string) string {
				return filepath.Join(tmp, "missing")
			},
			setup: func(t *testing.T, dir string) {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			dir := tmp
			if tc.dir != nil {
				dir = tc.dir(tmp)
			}
			tc.setup(t, dir)

			got, err := ListFiles(dir)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("ListFiles() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func writeArtifact(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
