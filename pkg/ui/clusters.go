package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	installertypes "github.com/openshift/installer/pkg/types"
	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
	clustertemplatev1alpha1 "github.com/openshift/test-harness-operator/pkg/api/clustertemplate/v1alpha1"
)

const (
	cloudCredentialsLabel = "harness.testharness.io/cloud-credentials"

	awsAccessKeyEnv = "AWS_ACCESS_KEY_ID"
	awsSecretKeyEnv = "AWS_SECRET_ACCESS_KEY"

	gcpCredentialsFile = "osServiceAccount.json"
	gcpCredentialsEnv  = "GOOGLE_APPLICATION_CREDENTIALS"

	azureCredentialsFile = "osServicePrincipal.json"
	azureCredentialsEnv  = "AZURE_AUTH_LOCATION"

	credentialsMountPath = "/credentials"
)

type clusterRow struct {
	Name        string
	Phase       string
	Installer   string
	Credentials string
	Age         string
}

type clusterDetail struct {
	Name          string
	Phase         string
	Message       string
	Installer     string
	Credentials   string
	InstallConfig string
	Created       string
}

type templateOption struct {
	Name        string
	Installer   string
	Credentials string
}

type templateDetail struct {
	Name          string
	Installer     string
	Credentials   string
	InstallConfig string
}

func (s *Server) handleClusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleClusterList(w, r)
	case http.MethodPost:
		s.handleClusterCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClusterList(w http.ResponseWriter, r *http.Request) {
	data := pageData{Section: "clusters"}
	var list clusterv1alpha1.ClusterList
	if err := s.client.List(r.Context(), &list, client.InNamespace(s.namespace)); err != nil {
		data.Error = err.Error()
		s.render(w, "cluster_list", data)
		return
	}
	now := time.Now()
	for i := range list.Items {
		cluster := &list.Items[i]
		data.Clusters = append(data.Clusters, clusterRow{
			Name:        cluster.Name,
			Phase:       cluster.Status.Phase,
			Installer:   cluster.Spec.InstallerImage,
			Credentials: cluster.Spec.CredentialsSecretRef.Name,
			Age:         ageString(now, cluster.CreationTimestamp.Time),
		})
	}
	s.render(w, "cluster_list", data)
}

func (s *Server) handleClusterCreateForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := pageData{Section: "clusters", SelectedTemplate: strings.TrimSpace(r.URL.Query().Get("template"))}
	if err := s.loadClusterForm(r.Context(), &data); err != nil {
		data.Error = err.Error()
	}
	s.render(w, "cluster_create", data)
}

func (s *Server) handleClusterCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	image := strings.TrimSpace(r.FormValue("installerImage"))
	secretName := strings.TrimSpace(r.FormValue("credentialsSecret"))
	rawConfig := strings.TrimSpace(r.FormValue("installConfig"))
	data := pageData{
		Section:           "clusters",
		SelectedTemplate:  strings.TrimSpace(r.FormValue("template")),
		InstallerImage:    image,
		CredentialsSecret: secretName,
		InstallConfigYAML: rawConfig,
	}
	if err := s.loadClusterForm(r.Context(), &data); err != nil {
		data.Error = err.Error()
		s.render(w, "cluster_create", data)
		return
	}
	cluster, err := s.clusterFromForm(r.Context(), name, image, secretName, rawConfig)
	if err != nil {
		data.Error = err.Error()
		s.render(w, "cluster_create", data)
		return
	}
	if err := s.client.Create(r.Context(), cluster); err != nil {
		data.Error = err.Error()
		s.render(w, "cluster_create", data)
		return
	}
	http.Redirect(w, r, "/clusters/"+name, http.StatusSeeOther)
}

func (s *Server) handleClusterPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/clusters/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if len(parts) == 1 {
		s.handleClusterDetail(w, r, name)
		return
	}
	if len(parts) == 2 && parts[1] == "delete" {
		s.handleClusterDelete(w, r, name)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleClusterDetail(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: s.namespace, Name: name}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, err := yaml.Marshal(cluster.Spec.InstallConfig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "cluster_detail", pageData{
		Section: "clusters",
		Cluster: &clusterDetail{
			Name:          cluster.Name,
			Phase:         cluster.Status.Phase,
			Message:       cluster.Status.Message,
			Installer:     cluster.Spec.InstallerImage,
			Credentials:   cluster.Spec.CredentialsSecretRef.Name,
			InstallConfig: string(raw),
			Created:       cluster.CreationTimestamp.Time.Format(time.RFC3339),
		},
	})
}

func (s *Server) handleClusterDelete(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cluster := &clusterv1alpha1.Cluster{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: s.namespace, Name: name}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			http.Redirect(w, r, "/clusters", http.StatusSeeOther)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.client.Delete(r.Context(), cluster); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/clusters/"+name, http.StatusSeeOther)
}

func (s *Server) handleCloudSecretCreateForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.render(w, "cloud_secret_create", pageData{Section: "clusters", CloudPlatform: "aws"})
}

func (s *Server) handleCloudSecrets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	platform := strings.TrimSpace(r.FormValue("platform"))
	secret, err := cloudCredentialSecret(s.namespace, strings.TrimSpace(r.FormValue("name")), platform, r.Form)
	if err != nil {
		s.render(w, "cloud_secret_create", pageData{Section: "clusters", Error: err.Error(), CloudPlatform: platform})
		return
	}
	if err := s.client.Create(r.Context(), secret); err != nil {
		s.render(w, "cloud_secret_create", pageData{Section: "clusters", Error: err.Error(), CloudPlatform: platform})
		return
	}
	http.Redirect(w, r, "/clusters/new", http.StatusSeeOther)
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleTemplateList(w, r)
	case http.MethodPost:
		s.handleTemplateCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTemplateList(w http.ResponseWriter, r *http.Request) {
	data := pageData{Section: "clusters"}
	if err := s.loadClusterForm(r.Context(), &data); err != nil {
		data.Error = err.Error()
	}
	s.render(w, "template_list", data)
}

func (s *Server) handleTemplateCreateForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := pageData{Section: "clusters"}
	if err := s.loadClusterForm(r.Context(), &data); err != nil {
		data.Error = err.Error()
	}
	s.render(w, "template_create", data)
}

func (s *Server) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	image := strings.TrimSpace(r.FormValue("installerImage"))
	secretName := strings.TrimSpace(r.FormValue("credentialsSecret"))
	rawConfig := strings.TrimSpace(r.FormValue("installConfig"))
	data := pageData{
		Section:           "clusters",
		InstallerImage:    image,
		CredentialsSecret: secretName,
		InstallConfigYAML: rawConfig,
	}
	if err := s.loadClusterForm(r.Context(), &data); err != nil {
		data.Error = err.Error()
		s.render(w, "template_create", data)
		return
	}
	item, err := s.templateFromForm(r.Context(), name, image, secretName, rawConfig)
	if err != nil {
		data.Error = err.Error()
		s.render(w, "template_create", data)
		return
	}
	if err := s.client.Create(r.Context(), item); err != nil {
		data.Error = err.Error()
		s.render(w, "template_create", data)
		return
	}
	http.Redirect(w, r, "/clusters/templates/"+name, http.StatusSeeOther)
}

func (s *Server) handleTemplatePath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/clusters/templates/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	if len(parts) == 1 {
		s.handleTemplateDetail(w, r, name)
		return
	}
	if len(parts) == 2 && parts[1] == "delete" {
		s.handleTemplateDelete(w, r, name)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleTemplateDetail(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item := &clustertemplatev1alpha1.ClusterTemplate{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: s.namespace, Name: name}, item); err != nil {
		if apierrors.IsNotFound(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	raw, err := yaml.Marshal(item.Spec.InstallConfig)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "template_detail", pageData{
		Section: "clusters",
		Template: &templateDetail{
			Name:          item.Name,
			Installer:     item.Spec.InstallerImage,
			Credentials:   item.Spec.CredentialsSecretRef.Name,
			InstallConfig: string(raw),
		},
	})
}

func (s *Server) handleTemplateDelete(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	item := &clustertemplatev1alpha1.ClusterTemplate{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Namespace: s.namespace, Name: name}, item); err != nil {
		if apierrors.IsNotFound(err) {
			http.Redirect(w, r, "/clusters/templates", http.StatusSeeOther)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.client.Delete(r.Context(), item); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/clusters/templates", http.StatusSeeOther)
}

func (s *Server) loadClusterForm(ctx context.Context, data *pageData) error {
	var secrets corev1.SecretList
	if err := s.client.List(ctx, &secrets, client.InNamespace(s.namespace), client.MatchingLabels{
		cloudCredentialsLabel: "true",
	}); err != nil {
		return err
	}
	data.CloudSecrets = secrets.Items

	var templates clustertemplatev1alpha1.ClusterTemplateList
	if err := s.client.List(ctx, &templates, client.InNamespace(s.namespace)); err != nil {
		return err
	}
	for i := range templates.Items {
		item := &templates.Items[i]
		data.Templates = append(data.Templates, templateOption{
			Name:        item.Name,
			Installer:   item.Spec.InstallerImage,
			Credentials: item.Spec.CredentialsSecretRef.Name,
		})
		if data.SelectedTemplate != item.Name || data.InstallConfigYAML != "" {
			continue
		}
		raw, err := yaml.Marshal(item.Spec.InstallConfig)
		if err != nil {
			return fmt.Errorf("render template %s install-config: %w", item.Name, err)
		}
		data.InstallerImage = item.Spec.InstallerImage
		data.CredentialsSecret = item.Spec.CredentialsSecretRef.Name
		data.InstallConfigYAML = string(raw)
	}
	return nil
}

func (s *Server) cloudSecretExists(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("credentials secret is required")
	}
	secret := &corev1.Secret{}
	if err := s.client.Get(ctx, client.ObjectKey{Namespace: s.namespace, Name: name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("credentials secret %s not found", name)
		}
		return err
	}
	if secret.Labels[cloudCredentialsLabel] != "true" {
		return fmt.Errorf("secret %s is not a cloud credentials secret", name)
	}
	return nil
}

func cloudCredentialSecret(namespace, name, platform string, form map[string][]string) (*corev1.Secret, error) {
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return nil, fmt.Errorf("name: %s", strings.Join(errs, ", "))
	}
	values := map[string]string{}
	for key, got := range form {
		if len(got) > 0 {
			values[key] = strings.TrimSpace(got[0])
		}
	}
	data := map[string]string{}
	switch platform {
	case "aws":
		accessKey := values["awsAccessKeyID"]
		secretKey := values["awsSecretAccessKey"]
		if accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("AWS access key ID and secret access key are required")
		}
		data[awsAccessKeyEnv] = accessKey
		data[awsSecretKeyEnv] = secretKey
	case "gcp":
		raw := values["gcpServiceAccountJSON"]
		if raw == "" {
			return nil, fmt.Errorf("GCP service account JSON is required")
		}
		if err := requireJSONObject(raw); err != nil {
			return nil, fmt.Errorf("GCP service account JSON: %w", err)
		}
		data[gcpCredentialsFile] = raw
		data[gcpCredentialsEnv] = credentialsMountPath + "/" + gcpCredentialsFile
	case "azure":
		raw := values["azureServicePrincipalJSON"]
		if raw == "" {
			return nil, fmt.Errorf("Azure service principal JSON is required")
		}
		if err := requireJSONObject(raw); err != nil {
			return nil, fmt.Errorf("Azure service principal JSON: %w", err)
		}
		data[azureCredentialsFile] = raw
		data[azureCredentialsEnv] = credentialsMountPath + "/" + azureCredentialsFile
	default:
		return nil, fmt.Errorf("platform must be aws, gcp, or azure")
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				cloudCredentialsLabel: "true",
			},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}, nil
}

func (s *Server) clusterFromForm(ctx context.Context, name, image, secretName, rawConfig string) (*clusterv1alpha1.Cluster, error) {
	cfg, err := s.installConfigForForm(ctx, name, secretName, rawConfig)
	if err != nil {
		return nil, err
	}
	return &clusterv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: clusterv1alpha1.ClusterSpec{
			InstallerImage:       image,
			InstallConfig:        *cfg,
			CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
		},
	}, nil
}

func (s *Server) templateFromForm(ctx context.Context, name, image, secretName, rawConfig string) (*clustertemplatev1alpha1.ClusterTemplate, error) {
	cfg, err := s.installConfigForForm(ctx, name, secretName, rawConfig)
	if err != nil {
		return nil, err
	}
	return &clustertemplatev1alpha1.ClusterTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: clusterv1alpha1.ClusterSpec{
			InstallerImage:       image,
			InstallConfig:        *cfg,
			CredentialsSecretRef: corev1.LocalObjectReference{Name: secretName},
		},
	}, nil
}

func (s *Server) installConfigForForm(ctx context.Context, name, secretName, rawConfig string) (*installertypes.InstallConfig, error) {
	cfg, err := installConfigFromYAML(name, rawConfig)
	if err != nil {
		return nil, err
	}
	if err := s.cloudSecretExists(ctx, secretName); err != nil {
		return nil, err
	}
	return cfg, nil
}

func installConfigFromYAML(clusterName, raw string) (*installertypes.InstallConfig, error) {
	if raw == "" {
		return nil, fmt.Errorf("install-config is required")
	}
	cfg := &installertypes.InstallConfig{}
	if err := yaml.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, fmt.Errorf("install-config: %w", err)
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = installertypes.InstallConfigVersion
	}
	if cfg.ObjectMeta.Name == "" {
		cfg.ObjectMeta.Name = clusterName
	}
	return cfg, nil
}

func requireJSONObject(raw string) error {
	var object map[string]any
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return fmt.Errorf("must be a JSON object")
	}
	if len(object) == 0 {
		return fmt.Errorf("must be a non-empty JSON object")
	}
	return nil
}
