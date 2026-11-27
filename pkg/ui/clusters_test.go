package ui

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
	installertypes "github.com/openshift/installer/pkg/types"
	awstypes "github.com/openshift/installer/pkg/types/aws"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
	clustertemplatev1alpha1 "github.com/openshift/test-harness-operator/pkg/api/clustertemplate/v1alpha1"
)

var uiScheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(uiScheme))
	utilruntime.Must(clusterv1alpha1.AddToScheme(uiScheme))
	utilruntime.Must(clustertemplatev1alpha1.AddToScheme(uiScheme))
}

func TestCloudCredentialSecret(t *testing.T) {
	testCases := []struct {
		name     string
		secret   string
		platform string
		form     url.Values
		want     *corev1.Secret
		wantErr  bool
	}{
		{
			name:     "aws keys",
			secret:   "aws-ci",
			platform: "aws",
			form: url.Values{
				"awsAccessKeyID":     {"AKIA"},
				"awsSecretAccessKey": {"secret"},
			},
			want: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-ci",
					Namespace: "test-harness",
					Labels:    map[string]string{cloudCredentialsLabel: "true"},
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"AWS_ACCESS_KEY_ID":     "AKIA",
					"AWS_SECRET_ACCESS_KEY": "secret",
				},
			},
		},
		{
			name:     "aws missing secret",
			secret:   "aws-ci",
			platform: "aws",
			form:     url.Values{"awsAccessKeyID": {"AKIA"}},
			wantErr:  true,
		},
		{
			name:     "gcp file and path",
			secret:   "gcp-ci",
			platform: "gcp",
			form:     url.Values{"gcpServiceAccountJSON": {`{"type":"service_account"}`}},
			want: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gcp-ci",
					Namespace: "test-harness",
					Labels:    map[string]string{cloudCredentialsLabel: "true"},
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"osServiceAccount.json":          `{"type":"service_account"}`,
					"GOOGLE_APPLICATION_CREDENTIALS": "/credentials/osServiceAccount.json",
				},
			},
		},
		{
			name:     "azure file and path",
			secret:   "azure-ci",
			platform: "azure",
			form:     url.Values{"azureServicePrincipalJSON": {`{"clientId":"id"}`}},
			want: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "azure-ci",
					Namespace: "test-harness",
					Labels:    map[string]string{cloudCredentialsLabel: "true"},
				},
				Type: corev1.SecretTypeOpaque,
				StringData: map[string]string{
					"osServicePrincipal.json": `{"clientId":"id"}`,
					"AZURE_AUTH_LOCATION":     "/credentials/osServicePrincipal.json",
				},
			},
		},
		{
			name:     "bad json",
			secret:   "gcp-ci",
			platform: "gcp",
			form:     url.Values{"gcpServiceAccountJSON": {"not-json"}},
			wantErr:  true,
		},
		{
			name:     "unknown platform",
			secret:   "creds",
			platform: "vsphere",
			wantErr:  true,
		},
		{
			name:     "bad name",
			secret:   "Bad_Name",
			platform: "aws",
			form: url.Values{
				"awsAccessKeyID":     {"AKIA"},
				"awsSecretAccessKey": {"secret"},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cloudCredentialSecret("test-harness", tc.secret, tc.platform, tc.form)
			if tc.wantErr {
				if err == nil {
					t.Fatal("cloudCredentialSecret() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("cloudCredentialSecret() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("cloudCredentialSecret() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInstallConfigFromYAML(t *testing.T) {
	testCases := []struct {
		name        string
		clusterName string
		raw         string
		want        *installertypes.InstallConfig
		wantErr     bool
	}{
		{
			name:        "fills empty name and api version",
			clusterName: "ci-aws",
			raw:         "baseDomain: example.com\n",
			want: &installertypes.InstallConfig{
				TypeMeta:   metav1.TypeMeta{APIVersion: installertypes.InstallConfigVersion},
				ObjectMeta: metav1.ObjectMeta{Name: "ci-aws"},
				BaseDomain: "example.com",
			},
		},
		{
			name:        "keeps metadata name",
			clusterName: "ci-aws",
			raw:         "metadata:\n  name: kept\nbaseDomain: example.com\n",
			want: &installertypes.InstallConfig{
				TypeMeta:   metav1.TypeMeta{APIVersion: installertypes.InstallConfigVersion},
				ObjectMeta: metav1.ObjectMeta{Name: "kept"},
				BaseDomain: "example.com",
			},
		},
		{
			name:    "empty",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "invalid",
			raw:     ":\n",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := installConfigFromYAML(tc.clusterName, tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("installConfigFromYAML() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("installConfigFromYAML() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("installConfigFromYAML() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestClusterFromForm(t *testing.T) {
	testCases := []struct {
		name       string
		objects    []client.Object
		objectName string
		image      string
		secretName string
		rawConfig  string
		want       *clusterv1alpha1.Cluster
		wantErr    bool
	}{
		{
			name: "posted install config",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-ci",
						Namespace: "test-harness",
						Labels:    map[string]string{cloudCredentialsLabel: "true"},
					},
				},
			},
			objectName: "ci-aws",
			image:      "installer:posted",
			secretName: "aws-ci",
			rawConfig:  "baseDomain: example.com\nplatform:\n  aws:\n    region: us-east-1\n",
			want: &clusterv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "ci-aws", Namespace: "test-harness"},
				Spec: clusterv1alpha1.ClusterSpec{
					InstallerImage: "installer:posted",
					InstallConfig: installertypes.InstallConfig{
						TypeMeta:   metav1.TypeMeta{APIVersion: installertypes.InstallConfigVersion},
						ObjectMeta: metav1.ObjectMeta{Name: "ci-aws"},
						BaseDomain: "example.com",
						Platform:   installertypes.Platform{AWS: &awstypes.Platform{Region: "us-east-1"}},
					},
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-ci"},
				},
			},
		},
		{
			name:       "missing credentials",
			objectName: "ci-aws",
			image:      "installer:posted",
			secretName: "aws-ci",
			rawConfig:  "baseDomain: example.com\n",
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{
				client:    fake.NewClientBuilder().WithScheme(uiScheme).WithObjects(tc.objects...).Build(),
				namespace: "test-harness",
			}
			got, err := server.clusterFromForm(context.Background(), tc.objectName, tc.image, tc.secretName, tc.rawConfig)
			if tc.wantErr {
				if err == nil {
					t.Fatal("clusterFromForm() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("clusterFromForm() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("clusterFromForm() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestTemplateFromForm(t *testing.T) {
	testCases := []struct {
		name       string
		objects    []client.Object
		objectName string
		image      string
		secretName string
		rawConfig  string
		want       *clustertemplatev1alpha1.ClusterTemplate
		wantErr    bool
	}{
		{
			name: "posted install config",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-ci",
						Namespace: "test-harness",
						Labels:    map[string]string{cloudCredentialsLabel: "true"},
					},
				},
			},
			objectName: "aws-default",
			image:      "installer:tpl",
			secretName: "aws-ci",
			rawConfig:  "baseDomain: example.com\n",
			want: &clustertemplatev1alpha1.ClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "aws-default", Namespace: "test-harness"},
				Spec: clusterv1alpha1.ClusterSpec{
					InstallerImage: "installer:tpl",
					InstallConfig: installertypes.InstallConfig{
						TypeMeta:   metav1.TypeMeta{APIVersion: installertypes.InstallConfigVersion},
						ObjectMeta: metav1.ObjectMeta{Name: "aws-default"},
						BaseDomain: "example.com",
					},
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-ci"},
				},
			},
		},
		{
			name: "secret is not cloud credentials",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "aws-ci", Namespace: "test-harness"},
				},
			},
			objectName: "aws-default",
			image:      "installer:tpl",
			secretName: "aws-ci",
			rawConfig:  "baseDomain: example.com\n",
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{
				client:    fake.NewClientBuilder().WithScheme(uiScheme).WithObjects(tc.objects...).Build(),
				namespace: "test-harness",
			}
			got, err := server.templateFromForm(context.Background(), tc.objectName, tc.image, tc.secretName, tc.rawConfig)
			if tc.wantErr {
				if err == nil {
					t.Fatal("templateFromForm() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("templateFromForm() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("templateFromForm() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadClusterForm(t *testing.T) {
	testCases := []struct {
		name    string
		objects []client.Object
		in      pageData
		want    pageData
		wantErr bool
	}{
		{
			name: "copies the selected template",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-ci",
						Namespace: "test-harness",
						Labels:    map[string]string{cloudCredentialsLabel: "true"},
					},
				},
				&clustertemplatev1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "test-harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "installer:kept",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-ci"},
						InstallConfig:        installertypes.InstallConfig{ObjectMeta: metav1.ObjectMeta{Name: "from-template"}},
					},
				},
			},
			in: pageData{SelectedTemplate: "aws"},
			want: pageData{
				SelectedTemplate:  "aws",
				InstallerImage:    "installer:kept",
				CredentialsSecret: "aws-ci",
				InstallConfigYAML: "baseDomain: \"\"\nmetadata:\n  name: from-template\nplatform: {}\npullSecret: \"\"\n",
				CloudSecrets: []corev1.Secret{{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "aws-ci",
						Namespace:       "test-harness",
						ResourceVersion: "999",
						Labels:          map[string]string{cloudCredentialsLabel: "true"},
					},
				}},
				Templates: []templateOption{{
					Name:        "aws",
					Installer:   "installer:kept",
					Credentials: "aws-ci",
				}},
			},
		},
		{
			name: "leaves a posted install config",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "aws-ci",
						Namespace: "test-harness",
						Labels:    map[string]string{cloudCredentialsLabel: "true"},
					},
				},
				&clustertemplatev1alpha1.ClusterTemplate{
					ObjectMeta: metav1.ObjectMeta{Name: "aws", Namespace: "test-harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "installer:kept",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-ci"},
						InstallConfig:        installertypes.InstallConfig{ObjectMeta: metav1.ObjectMeta{Name: "from-template"}},
					},
				},
			},
			in: pageData{
				SelectedTemplate:  "aws",
				InstallerImage:    "installer:posted",
				InstallConfigYAML: "baseDomain: posted\n",
			},
			want: pageData{
				SelectedTemplate:  "aws",
				InstallerImage:    "installer:posted",
				InstallConfigYAML: "baseDomain: posted\n",
				CloudSecrets: []corev1.Secret{{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "aws-ci",
						Namespace:       "test-harness",
						ResourceVersion: "999",
						Labels:          map[string]string{cloudCredentialsLabel: "true"},
					},
				}},
				Templates: []templateOption{{
					Name:        "aws",
					Installer:   "installer:kept",
					Credentials: "aws-ci",
				}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{
				client:    fake.NewClientBuilder().WithScheme(uiScheme).WithObjects(tc.objects...).Build(),
				namespace: "test-harness",
			}
			got := tc.in
			err := server.loadClusterForm(context.Background(), &got)
			if tc.wantErr {
				if err == nil {
					t.Fatal("loadClusterForm() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("loadClusterForm() error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("loadClusterForm() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
