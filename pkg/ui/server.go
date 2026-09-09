package ui

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	reliabilitytestv1alpha1 "github.com/droslean/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/droslean/test-harness-operator/pkg/controller"
)

//go:embed templates/*.html
var templateFS embed.FS

const managedByTestHarnessOperatorLabel = "harness.testharness.io/managed-by-test-harness-operator"

// Server serves the test-harness management UI.
type Server struct {
	client    client.WithWatch
	kube      kubernetes.Interface
	namespace string
	pages     map[string]*template.Template
}

// New returns a UI server for the given namespace.
func New(cfg *rest.Config, c client.WithWatch, namespace string) (*Server, error) {
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	s := &Server{
		client:    c,
		kube:      kube,
		namespace: namespace,
		pages:     map[string]*template.Template{},
	}
	for _, name := range []string{"list", "create", "detail", "logs", "secret_list", "secret_create"} {
		t, err := template.ParseFS(templateFS, "templates/base.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("parse %s templates: %w", name, err)
		}
		s.pages[name] = t
	}
	return s, nil
}

// Handler returns the HTTP handler for the UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleList)
	mux.HandleFunc("/tests/new", s.handleCreateForm)
	mux.HandleFunc("/tests", s.handleTests)
	mux.HandleFunc("/tests/", s.handleTestPath)
	mux.HandleFunc("/secrets/new", s.handleSecretCreateForm)
	mux.HandleFunc("/secrets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.handleSecretList(w, r)
		case http.MethodPost:
			s.handleSecrets(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/tests", s.handleAPITests)
	mux.HandleFunc("/api/tests/", s.handleAPITest)
	mux.HandleFunc("/watch/tests", s.handleWatchTests)
	mux.HandleFunc("/watch/tests/", s.handleWatchTest)
	return mux
}

// Start listens on addr until ctx is cancelled.
func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	logrus.WithField("addr", addr).Info("starting UI server")
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

type pageData struct {
	Section     string
	Namespace   string
	Error       string
	Tests       []testRow
	SecretRows  []secretRow
	AuthSecrets []corev1.Secret
	Test        *testDetail
	Pod         *podDetail
	ExpectedPod string
	TestName    string
	PodName     string
}

type testRow struct {
	Name     string `json:"name"`
	Scenario string `json:"scenario"`
	Phase    string `json:"phase"`
	PodName  string `json:"podName"`
	Age      string `json:"age"`
}

type secretRow struct {
	Name string
	Keys string
	Age  string
}

type testDetail struct {
	Name            string `json:"name"`
	Phase           string `json:"phase"`
	Scenario        string `json:"scenario"`
	Duration        string `json:"duration"`
	Image           string `json:"image"`
	AuthSecret      string `json:"authSecret"`
	ToleranceRate   string `json:"toleranceRate"`
	FolderName      string `json:"folderName"`
	Operators       string `json:"operators"`
	Infra           bool   `json:"infra"`
	Upgrade         bool   `json:"upgrade"`
	ClusterTopology string `json:"clusterTopology"`
	ImportDashboard string `json:"importDashboard"`
	SlackMember     string `json:"slackMember"`
	SlackChannel    string `json:"slackChannel"`
	SlackAPIToken   string `json:"slackAPIToken"`
	SlackWebhook    string `json:"slackWebhook"`
	Message         string `json:"message"`
	Created         string `json:"created"`
}

type podDetail struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Node    string `json:"node"`
	Started string `json:"started"`
}

type testStatusJSON struct {
	Test        *testDetail `json:"test"`
	Pod         *podDetail  `json:"pod,omitempty"`
	ExpectedPod string      `json:"expectedPod"`
}

func (s *Server) render(w http.ResponseWriter, page string, data pageData) {
	data.Namespace = s.namespace
	if data.Section == "" {
		data.Section = "tests"
	}
	t := s.pages[page]
	if t == nil {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		logrus.WithError(err).Error("render template")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := pageData{}
	var list reliabilitytestv1alpha1.ReliabilityTestList
	if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace)); err != nil {
		data.Error = err.Error()
		s.render(w, "list", data)
		return
	}
	now := time.Now()
	for i := range list.Items {
		t := &list.Items[i]
		data.Tests = append(data.Tests, testRow{
			Name:     t.Name,
			Scenario: t.Spec.Scenario,
			Phase:    t.Status.Phase,
			PodName:  t.Status.PodName,
			Age:      ageString(now, t.CreationTimestamp.Time),
		})
	}
	s.render(w, "list", data)
}

func (s *Server) handleCreateForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := pageData{}
	var list corev1.SecretList
	if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace), client.MatchingLabels{
		managedByTestHarnessOperatorLabel: "true",
	}); err != nil {
		data.Error = err.Error()
	} else {
		data.AuthSecrets = list.Items
	}
	s.render(w, "create", data)
}

func (s *Server) handleSecretCreateForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.render(w, "secret_create", pageData{Section: "secrets"})
}

func (s *Server) handleSecretList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := pageData{Section: "secrets"}
	var list corev1.SecretList
	if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace), client.MatchingLabels{
		managedByTestHarnessOperatorLabel: "true",
	}); err != nil {
		data.Error = err.Error()
		s.render(w, "secret_list", data)
		return
	}
	now := time.Now()
	for i := range list.Items {
		secret := &list.Items[i]
		data.SecretRows = append(data.SecretRows, secretRow{
			Name: secret.Name,
			Keys: authSecretKeys(secret),
			Age:  ageString(now, secret.CreationTimestamp.Time),
		})
	}
	s.render(w, "secret_list", data)
}

func authSecretKeys(secret *corev1.Secret) string {
	want := []string{"kubeconfig", "admin", "users"}
	var keys []string
	for _, k := range want {
		if _, ok := secret.Data[k]; ok {
			keys = append(keys, k)
		}
	}
	return strings.Join(keys, ", ")
}

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	kubeconfig := strings.TrimSpace(r.FormValue("kubeconfig"))
	admin := strings.TrimSpace(r.FormValue("admin"))
	users := strings.TrimSpace(r.FormValue("users"))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
			Labels: map[string]string{
				managedByTestHarnessOperatorLabel: "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"kubeconfig": kubeconfig,
			"admin":      admin,
			"users":      users,
		},
	}
	if err := s.client.Create(r.Context(), secret); err != nil {
		s.render(w, "secret_create", pageData{Section: "secrets", Error: err.Error()})
		return
	}
	http.Redirect(w, r, "/secrets", http.StatusSeeOther)
}

func (s *Server) handleTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	scenario := strings.TrimSpace(r.FormValue("scenario"))
	authSecret := strings.TrimSpace(r.FormValue("authSecret"))
	test := &reliabilitytestv1alpha1.ReliabilityTest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: reliabilitytestv1alpha1.ReliabilityTestSpec{
			Scenario:         scenario,
			AuthSecretRef:    corev1.LocalObjectReference{Name: authSecret},
			Duration:         strings.TrimSpace(r.FormValue("duration")),
			Image:            strings.TrimSpace(r.FormValue("image")),
			ToleranceRate:    strings.TrimSpace(r.FormValue("toleranceRate")),
			FolderName:       strings.TrimSpace(r.FormValue("folderName")),
			Operators:        strings.TrimSpace(r.FormValue("operators")),
			Infra:            r.FormValue("infra") == "true",
			Upgrade:          r.FormValue("upgrade") == "true",
			ClusterTopology:  strings.TrimSpace(r.FormValue("clusterTopology")),
			ImportDashboard:  strings.TrimSpace(r.FormValue("importDashboard")),
			Slack:            slackSpecFromForm(r),
		},
	}
	if err := s.client.Create(r.Context(), test); err != nil {
		data := pageData{Error: err.Error()}
		var list corev1.SecretList
		if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace), client.MatchingLabels{
			managedByTestHarnessOperatorLabel: "true",
		}); err == nil {
			data.AuthSecrets = list.Items
		}
		s.render(w, "create", data)
		return
	}
	http.Redirect(w, r, "/tests/"+name, http.StatusSeeOther)
}

func (s *Server) handleTestPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tests/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if len(parts) == 1 {
		s.handleDetail(w, r, name)
		return
	}
	switch parts[1] {
	case "delete":
		s.handleDelete(w, r, name)
	case "logs":
		if len(parts) >= 3 && parts[2] == "stream" {
			s.handleLogsStream(w, r, name)
			return
		}
		s.handleLogs(w, r, name)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request, name string) {
	status, err := s.loadTestStatus(r.Context(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		s.render(w, "detail", pageData{Error: err.Error(), ExpectedPod: controller.RunnerPodName(name)})
		return
	}
	s.render(w, "detail", pageData{
		Test:        status.Test,
		Pod:         status.Pod,
		ExpectedPod: status.ExpectedPod,
	})
}

func (s *Server) handleAPITests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var list reliabilitytestv1alpha1.ReliabilityTestList
	if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now()
	rows := make([]testRow, 0, len(list.Items))
	for i := range list.Items {
		t := &list.Items[i]
		rows = append(rows, testRow{
			Name:     t.Name,
			Scenario: t.Spec.Scenario,
			Phase:    t.Status.Phase,
			PodName:  t.Status.PodName,
			Age:      ageString(now, t.CreationTimestamp.Time),
		})
	}
	writeJSON(w, rows)
}

func (s *Server) handleAPITest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tests/"), "/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	status, err := s.loadTestStatus(r.Context(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, status)
}

func (s *Server) loadTestStatus(ctx context.Context, name string) (*testStatusJSON, error) {
	test := &reliabilitytestv1alpha1.ReliabilityTest{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: s.namespace, Name: name}, test); err != nil {
		return nil, err
	}
	detail := &testDetail{
		Name:            test.Name,
		Phase:           test.Status.Phase,
		Scenario:        test.Spec.Scenario,
		Duration:        test.Spec.Duration,
		Image:           test.Spec.Image,
		AuthSecret:      test.Spec.AuthSecretRef.Name,
		ToleranceRate:   test.Spec.ToleranceRate,
		FolderName:      test.Spec.FolderName,
		Operators:       test.Spec.Operators,
		Infra:           test.Spec.Infra,
		Upgrade:         test.Spec.Upgrade,
		ClusterTopology: test.Spec.ClusterTopology,
		ImportDashboard: test.Spec.ImportDashboard,
		Message:         test.Status.Message,
		Created:         test.CreationTimestamp.Format(time.RFC3339),
	}
	if test.Spec.Slack != nil {
		detail.SlackMember = test.Spec.Slack.Member
		detail.SlackChannel = test.Spec.Slack.Channel
		detail.SlackAPIToken = secretKeyRefString(test.Spec.Slack.APITokenSecretRef)
		detail.SlackWebhook = secretKeyRefString(test.Spec.Slack.WebhookURLSecretRef)
	}
	status := &testStatusJSON{
		Test:        detail,
		ExpectedPod: controller.RunnerPodName(name),
	}
	podName := test.Status.PodName
	if podName == "" {
		podName = status.ExpectedPod
	}
	pod := &corev1.Pod{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: s.namespace, Name: podName}, pod); err == nil {
		status.Pod = &podDetail{
			Name:    pod.Name,
			Phase:   string(pod.Status.Phase),
			Reason:  pod.Status.Reason,
			Message: pod.Status.Message,
			Node:    pod.Spec.NodeName,
			Started: formatTime(pod.Status.StartTime),
		}
	} else if !apierrors.IsNotFound(err) {
		return nil, err
	}
	return status, nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	test := &reliabilitytestv1alpha1.ReliabilityTest{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: s.namespace, Name: name}, test); err != nil {
		if apierrors.IsNotFound(err) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.client.Delete(r.Context(), test); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request, name string) {
	s.render(w, "logs", pageData{
		TestName: name,
		PodName:  controller.RunnerPodName(name),
	})
}

func (s *Server) handleLogsStream(w http.ResponseWriter, r *http.Request, name string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	podName := controller.RunnerPodName(name)
	opts := &corev1.PodLogOptions{
		Container: "reliability",
		Follow:    true,
		TailLines: int64Ptr(500),
	}
	stream, err := s.kube.CoreV1().Pods(s.namespace).GetLogs(podName, opts).Stream(r.Context())
	if err != nil {
		opts.Follow = false
		opts.Previous = true
		stream, err = s.kube.CoreV1().Pods(s.namespace).GetLogs(podName, opts).Stream(r.Context())
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	buf := make([]byte, 4096)
	for {
		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			flusher.Flush()
		}
		if readErr != nil {
			if readErr != io.EOF {
				logrus.WithError(readErr).WithField("pod", podName).Debug("log stream ended")
			}
			return
		}
	}
}

func (s *Server) handleWatchTests(w http.ResponseWriter, r *http.Request) {
	s.streamReliabilityWatch(w, r, nil)
}

func (s *Server) handleWatchTest(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/watch/tests/"), "/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	testWatch, err := s.client.Watch(r.Context(), &reliabilitytestv1alpha1.ReliabilityTestList{}, client.InNamespace(s.namespace))
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer testWatch.Stop()

	podWatch, err := s.kube.CoreV1().Pods(s.namespace).Watch(r.Context(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + controller.RunnerPodName(name),
	})
	if err != nil {
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer podWatch.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-testWatch.ResultChan():
			if !ok {
				return
			}
			if event.Type == watch.Error || event.Type == watch.Bookmark {
				continue
			}
			if t, ok := event.Object.(*reliabilitytestv1alpha1.ReliabilityTest); ok && t.Name != name {
				continue
			}
			fmt.Fprintf(w, "data: update\n\n")
			flusher.Flush()
		case event, ok := <-podWatch.ResultChan():
			if !ok {
				return
			}
			if event.Type == watch.Error || event.Type == watch.Bookmark {
				continue
			}
			fmt.Fprintf(w, "data: update\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) streamReliabilityWatch(w http.ResponseWriter, r *http.Request, opts []client.ListOption) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	listOpts := append([]client.ListOption{client.InNamespace(s.namespace)}, opts...)
	watcher, err := s.client.Watch(r.Context(), &reliabilitytestv1alpha1.ReliabilityTestList{}, listOpts...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer watcher.Stop()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			if event.Type == watch.Error || event.Type == watch.Bookmark {
				continue
			}
			fmt.Fprintf(w, "data: update\n\n")
			flusher.Flush()
		}
	}
}

func slackSpecFromForm(r *http.Request) *reliabilitytestv1alpha1.SlackSpec {
	member := strings.TrimSpace(r.FormValue("slackMember"))
	channel := strings.TrimSpace(r.FormValue("slackChannel"))
	apiSecret := strings.TrimSpace(r.FormValue("slackAPITokenSecret"))
	apiKey := strings.TrimSpace(r.FormValue("slackAPITokenKey"))
	webhookSecret := strings.TrimSpace(r.FormValue("slackWebhookSecret"))
	webhookKey := strings.TrimSpace(r.FormValue("slackWebhookKey"))

	hasAPIToken := apiSecret != "" && apiKey != ""
	hasWebhook := webhookSecret != "" && webhookKey != ""
	if member == "" && channel == "" && !hasAPIToken && !hasWebhook {
		return nil
	}
	slack := &reliabilitytestv1alpha1.SlackSpec{
		Member:  member,
		Channel: channel,
	}
	if hasAPIToken {
		slack.APITokenSecretRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: apiSecret},
			Key:                  apiKey,
		}
	}
	if hasWebhook {
		slack.WebhookURLSecretRef = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: webhookSecret},
			Key:                  webhookKey,
		}
	}
	return slack
}

func secretKeyRefString(ref *corev1.SecretKeySelector) string {
	if ref == nil || ref.Name == "" || ref.Key == "" {
		return ""
	}
	return ref.Name + "/" + ref.Key
}

func ageString(now, created time.Time) string {
	d := now.Sub(created).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func formatTime(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func int64Ptr(v int64) *int64 { return &v }
