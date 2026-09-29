package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/openshift/test-harness-operator/pkg/artifacts"
	"github.com/openshift/test-harness-operator/pkg/runner"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	code := run(ctx)
	os.Exit(code)
}

func run(ctx context.Context) int {
	cfg, err := fromEnv()
	if err != nil {
		logrus.WithError(err).Error("runner configuration")
		return 1
	}
	client, err := artifacts.NewClient(ctx, cfg.bucket, cfg.namespace, cfg.testName, cfg.podUID)
	if err != nil {
		logrus.WithError(err).Error("create gcs client")
		return 1
	}
	defer func() {
		if err := client.Close(); err != nil {
			logrus.WithError(err).Error("close gcs client")
		}
	}()

	return runner.Run(ctx, runner.Options{
		SuiteDir: artifacts.SuiteDir,
		Metadata: []byte(cfg.metadata),
		PodUID:   cfg.podUID,
		Upload: func(ctx context.Context, dir string) error {
			return client.Upload(context.WithoutCancel(ctx), dir)
		},
	})
}

type config struct {
	bucket    string
	testName  string
	namespace string
	podUID    string
	metadata  string
}

func fromEnv() (config, error) {
	cfg := config{}
	var err error
	if cfg.bucket, err = requiredEnv(artifacts.EnvBucket); err != nil {
		return config{}, err
	}
	if cfg.testName, err = requiredEnv(artifacts.EnvTestName); err != nil {
		return config{}, err
	}
	if cfg.namespace, err = requiredEnv(artifacts.EnvPodNamespace); err != nil {
		return config{}, err
	}
	if cfg.podUID, err = requiredEnv(artifacts.EnvPodUID); err != nil {
		return config{}, err
	}
	if cfg.metadata, err = requiredEnv(runner.EnvMetadata); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func requiredEnv(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("required environment variable %s was unset", key)
	}
	return value, nil
}
