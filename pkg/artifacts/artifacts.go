package artifacts

import (
	"os"
	"path/filepath"
	"sort"
)

const (
	// TestContainer is the container that runs the test.
	TestContainer = "reliability"
	// RunnerBinary is the command inside the reliability image.
	RunnerBinary = "/usr/local/bin/reliability-runner"
	// SuiteDir is the reliability-v2 working directory.
	SuiteDir = "/reliability-v2"

	// StdoutLog is the tee destination for start.sh stdout and stderr.
	StdoutLog = "stdout.log"
	// StartedFile is the test record stored in the suite output folder.
	StartedFile = "started.json"
	// FinishedFile is the exit record stored in the suite output folder.
	FinishedFile = "finished.json"

	// EnvBucket is the GCS bucket name.
	EnvBucket = "GCS_BUCKET"
	// EnvTestName is the ReliabilityTest name.
	EnvTestName = "TEST_NAME"
	// EnvPodNamespace is the runner pod namespace, from the downward API.
	EnvPodNamespace = "POD_NAMESPACE"
	// EnvPodUID is the runner pod UID, from the downward API.
	EnvPodUID = "POD_UID"

	// CredentialsKey is the Secret key that holds the GCP service account JSON.
	CredentialsKey = "service-account.json"
	// CredentialsMount is the mount path for the credentials Secret.
	CredentialsMount = "/gcs"
	// CredentialsFile is the service account JSON file.
	CredentialsFile = "/gcs/service-account.json"
)

// ListFiles walks root and returns regular files. Symlinks are skipped.
// A missing root is an empty result.
func ListFiles(root string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(root, func(name string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) && name == root {
				return nil
			}
			return walkErr
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, relErr := filepath.Rel(root, name)
		if relErr != nil {
			return relErr
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}
