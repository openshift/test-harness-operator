package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

func TestStartShArgs(t *testing.T) {
	const authPath = "/auth"

	testCases := []struct {
		name string
		spec ReliabilityTestSpec
		want []string
	}{
		{
			name: "required only",
			spec: ReliabilityTestSpec{
				Scenario: "reliability-small-cluster.yaml",
			},
			want: []string{"-p", "/auth", "-c", "reliability-small-cluster.yaml"},
		},
		{
			name: "all flags",
			spec: ReliabilityTestSpec{
				Scenario:      "reliability.yaml",
				Duration:      "2d",
				ToleranceRate: "5",
				FolderName:    "run1",
				Operators:     "netobserv-operator",
				Infra:         true,
				Upgrade:       true,
			},
			want: []string{
				"-p", "/auth",
				"-c", "reliability.yaml",
				"-t", "2d",
				"-r", "5",
				"-n", "run1",
				"-o", "netobserv-operator",
				"-i",
				"-u",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.StartShArgs(authPath)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("StartShArgs() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStartShEnv(t *testing.T) {
	const authPath = "/auth"

	apiTokenRef := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "slack"},
		Key:                  "api-token",
	}
	webhookRef := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: "slack"},
		Key:                  "webhook-url",
	}

	testCases := []struct {
		name string
		spec ReliabilityTestSpec
		want []corev1.EnvVar
	}{
		{
			name: "kubeconfig only",
			spec: ReliabilityTestSpec{},
			want: []corev1.EnvVar{
				{Name: EnvKubeconfig, Value: "/auth/kubeconfig"},
			},
		},
		{
			name: "all env",
			spec: ReliabilityTestSpec{
				ClusterTopology: "small",
				ImportDashboard: "/path/dashboard.json",
				Slack: &SlackSpec{
					Member:              "U123",
					Channel:             "C456",
					APITokenSecretRef:   apiTokenRef,
					WebhookURLSecretRef: webhookRef,
				},
			},
			want: []corev1.EnvVar{
				{Name: EnvKubeconfig, Value: "/auth/kubeconfig"},
				{Name: EnvClusterTopology, Value: "small"},
				{Name: EnvImportDashboard, Value: "/path/dashboard.json"},
				{Name: EnvSlackMember, Value: "U123"},
				{Name: EnvSlackChannel, Value: "C456"},
				{
					Name: EnvSlackAPIToken,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: apiTokenRef,
					},
				},
				{
					Name: EnvSlackWebhookURL,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: webhookRef,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.spec.StartShEnv(authPath)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("StartShEnv() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
