package ui

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/openshift/test-harness-operator/pkg/artifacts"
	"github.com/openshift/test-harness-operator/pkg/controller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func (s *Server) handleLogFiles(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir, err := s.suiteOutputDir(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out, err := s.exec(r.Context(), controller.RunnerPodName(name), []string{
		"find", dir, "-maxdepth", "1", "-type", "f", "-printf", "%f\n",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, logFileNames(string(out)))
}

func (s *Server) handleLogFile(w http.ResponseWriter, r *http.Request, name, fileName string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base, err := logFileName(fileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dir, err := s.suiteOutputDir(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out, err := s.exec(r.Context(), controller.RunnerPodName(name), []string{"cat", path.Join(dir, base)})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) suiteOutputDir(ctx context.Context, testName string) (string, error) {
	out, err := s.exec(ctx, controller.RunnerPodName(testName), []string{
		"find", artifacts.SuiteDir, "-mindepth", "2", "-maxdepth", "2", "-name", "reliability.log", "-print", "-quit",
	})
	if err != nil {
		return "", err
	}
	return logDirFromFind(string(out))
}

func logDirFromFind(out string) (string, error) {
	line := strings.TrimSpace(out)
	if line == "" {
		return "", fmt.Errorf("log directory not found")
	}
	dir := path.Dir(line)
	if dir == "." || dir == "/" || dir == artifacts.SuiteDir {
		return "", fmt.Errorf("log directory not found")
	}
	return dir, nil
}

func logFileName(name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid log file name %q", name)
	}
	return name, nil
}

func logFileNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if _, err := logFileName(name); err != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if names == nil {
		return []string{}
	}
	return names
}

func (s *Server) exec(ctx context.Context, podName string, command []string) ([]byte, error) {
	req := s.kube.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(s.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: artifacts.TestContainer,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(s.config, http.MethodPost, req.URL())
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, msg)
	}
	return stdout.Bytes(), nil
}
