package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// EnvOptions is the JSON the operator sets from the ReliabilityTest.
	EnvOptions = "RUNNER_OPTIONS"
	// EnvMetadata is the test record JSON the operator sets from the ReliabilityTest.
	EnvMetadata = "TEST_METADATA"
	// EnvBucket is the GCS bucket name.
	EnvBucket = "GCS_BUCKET"
	// EnvTestName is the ReliabilityTest name.
	EnvTestName = "TEST_NAME"
	// EnvPodNamespace is the runner pod namespace, from the downward API.
	EnvPodNamespace = "POD_NAMESPACE"
	// EnvPodUID is the runner pod UID, from the downward API.
	EnvPodUID = "POD_UID"

	// SuiteDir is the reliability-v2 working directory.
	SuiteDir = "/reliability-v2"

	// CredentialsKey is the Secret key that holds the GCS service account JSON.
	CredentialsKey = "service-account.json"
	// CredentialsMount is where the runner pod mounts that Secret.
	CredentialsMount = "/gcs"
	// CredentialsFile is the service account JSON the runner reads.
	CredentialsFile = "/gcs/service-account.json"

	stdoutLog    = "stdout.log"
	startedFile  = "started.json"
	finishedFile = "finished.json"
)

// Metadata is the test record written to started.json.
// The operator fills it from the ReliabilityTest. The runner adds the timestamp and pod UID.
type Metadata struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	PodName         string `json:"podName"`
	Scenario        string `json:"scenario"`
	Image           string `json:"image"`
	AuthSecret      string `json:"authSecret"`
	Duration        string `json:"duration,omitempty"`
	ToleranceRate   string `json:"toleranceRate,omitempty"`
	FolderName      string `json:"folderName,omitempty"`
	Operators       string `json:"operators,omitempty"`
	Infra           bool   `json:"infra"`
	Upgrade         bool   `json:"upgrade"`
	ClusterTopology string `json:"clusterTopology,omitempty"`
	ImportDashboard string `json:"importDashboard,omitempty"`
	Timestamp       int64  `json:"timestamp,omitempty"`
	PodUID          string `json:"podUID,omitempty"`
}

// StartOptions is the start.sh configuration the operator stores in EnvOptions.
type StartOptions struct {
	AuthPath      string `json:"authPath"`
	Scenario      string `json:"scenario"`
	Duration      string `json:"duration,omitempty"`
	ToleranceRate string `json:"toleranceRate,omitempty"`
	FolderName    string `json:"folderName,omitempty"`
	Operators     string `json:"operators,omitempty"`
	Infra         bool   `json:"infra,omitempty"`
	Upgrade       bool   `json:"upgrade,omitempty"`
}

// Args returns the start.sh arguments for these options.
func (o StartOptions) Args() []string {
	args := []string{"-p", o.AuthPath, "-c", o.Scenario}
	if o.Duration != "" {
		args = append(args, "-t", o.Duration)
	}
	if o.ToleranceRate != "" {
		args = append(args, "-r", o.ToleranceRate)
	}
	if o.FolderName != "" {
		args = append(args, "-n", o.FolderName)
	}
	if o.Operators != "" {
		args = append(args, "-o", o.Operators)
	}
	if o.Infra {
		args = append(args, "-i")
	}
	if o.Upgrade {
		args = append(args, "-u")
	}
	return args
}

func startOptions() (StartOptions, error) {
	var opt StartOptions
	if err := json.Unmarshal([]byte(os.Getenv(EnvOptions)), &opt); err != nil {
		return StartOptions{}, fmt.Errorf("parse %s: %w", EnvOptions, err)
	}
	return opt, nil
}

// Options is one reliability run.
type Options struct {
	SuiteDir string
	Metadata []byte
	PodUID   string
	Stdout   io.Writer
	Now      func() time.Time
	Upload   func(ctx context.Context, dir string) error
}

// Run runs start.sh, writes the runner records into the suite output folder, and uploads that folder.
// The returned code is the start.sh exit code. An upload failure does not change it.
func Run(ctx context.Context, opt Options) int {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	started, err := startedDocument(opt)
	if err != nil {
		logrus.WithError(err).Error("read test metadata")
		return 1
	}
	code, stdoutPath := runCommand(ctx, opt)
	dir, err := suiteLogDir(opt.SuiteDir)
	if err != nil {
		logrus.WithError(err).Error("find suite output")
		return code
	}
	if err := writeJSON(filepath.Join(dir, startedFile), started); err != nil {
		logrus.WithError(err).Error("write started.json")
	}
	if stdoutPath != "" {
		if err := moveFile(stdoutPath, filepath.Join(dir, stdoutLog)); err != nil {
			logrus.WithError(err).Error("store stdout log")
		}
	}
	if err := writeFinished(dir, opt.Now().Unix(), code); err != nil {
		logrus.WithError(err).Error("write finished.json")
	}
	if opt.Upload != nil {
		if err := opt.Upload(ctx, dir); err != nil {
			logrus.WithError(err).Error("upload artifacts")
		}
	}
	return code
}

func startedDocument(opt Options) (Metadata, error) {
	var doc Metadata
	if err := json.Unmarshal(opt.Metadata, &doc); err != nil {
		return Metadata{}, err
	}
	doc.Timestamp = opt.Now().Unix()
	if opt.PodUID != "" {
		doc.PodUID = opt.PodUID
	}
	return doc, nil
}

// Finished is the record written to finished.json.
type Finished struct {
	Timestamp int64 `json:"timestamp"`
	ExitCode  int   `json:"exitCode"`
}

func writeFinished(dir string, timestamp int64, code int) error {
	return writeJSON(filepath.Join(dir, finishedFile), Finished{
		Timestamp: timestamp,
		ExitCode:  code,
	})
}

func writeJSON(path string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func runCommand(ctx context.Context, opt Options) (int, string) {
	script, err := startOptions()
	if err != nil {
		logrus.WithError(err).Error("read runner options")
		return 1, ""
	}
	logFile, err := os.CreateTemp(opt.SuiteDir, ".stdout-")
	if err != nil {
		logrus.WithError(err).Error("create stdout log")
		return 1, ""
	}
	stdoutPath := logFile.Name()
	defer logFile.Close()

	cmd := exec.CommandContext(ctx, filepath.Join(opt.SuiteDir, "start.sh"), script.Args()...)
	cmd.Dir = opt.SuiteDir
	cmd.Stdout = io.MultiWriter(opt.Stdout, logFile)
	cmd.Stderr = cmd.Stdout
	err = cmd.Run()
	if err == nil {
		return 0, stdoutPath
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stdoutPath
	}
	logrus.WithError(err).Error("start.sh failed to start")
	return 1, stdoutPath
}

func suiteLogDir(suiteDir string) (string, error) {
	entries, err := os.ReadDir(suiteDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		logPath := filepath.Join(suiteDir, entry.Name(), "reliability.log")
		if _, err := os.Stat(logPath); err == nil {
			return filepath.Dir(logPath), nil
		}
	}
	return "", os.ErrNotExist
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", filepath.Base(src), err)
	}
	return out.Close()
}
