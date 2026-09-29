package controller

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	reliabilitytestv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/openshift/test-harness-operator/pkg/artifacts"
	"github.com/openshift/test-harness-operator/pkg/runner"
)

func TestBuildRunnerPod(t *testing.T) {
	reconciler := &ReliabilityTestReconciler{
		GCSBucket:            "test-bucket",
		GCSCredentialsSecret: "gcs-sa",
	}

	testCases := []struct {
		name    string
		test    *reliabilitytestv1alpha1.ReliabilityTest
		podName string
		env     []corev1.EnvVar
	}{
		{
			name:    "required fields",
			podName: "small-runner",
			test: &reliabilitytestv1alpha1.ReliabilityTest{
				ObjectMeta: metav1.ObjectMeta{Name: "small", Namespace: "harness"},
				Spec: reliabilitytestv1alpha1.ReliabilityTestSpec{
					Scenario:      "reliability-small-cluster.yaml",
					Image:         "quay.io/example/reliability:latest",
					AuthSecretRef: corev1.LocalObjectReference{Name: "cluster-auth"},
				},
			},
			env: []corev1.EnvVar{
				{Name: reliabilitytestv1alpha1.EnvKubeconfig, Value: "/auth/kubeconfig"},
			},
		},
		{
			name:    "start arguments preserved",
			podName: "full-runner",
			test: &reliabilitytestv1alpha1.ReliabilityTest{
				ObjectMeta: metav1.ObjectMeta{Name: "full", Namespace: "harness"},
				Spec: reliabilitytestv1alpha1.ReliabilityTestSpec{
					Scenario:        "reliability.yaml",
					Image:           "quay.io/example/reliability:latest",
					AuthSecretRef:   corev1.LocalObjectReference{Name: "cluster-auth"},
					Duration:        "2d",
					ToleranceRate:   "5",
					FolderName:      "run1",
					Operators:       "netobserv-operator",
					Infra:           true,
					Upgrade:         true,
					ClusterTopology: "small",
				},
			},
			env: []corev1.EnvVar{
				{Name: reliabilitytestv1alpha1.EnvKubeconfig, Value: "/auth/kubeconfig"},
				{Name: reliabilitytestv1alpha1.EnvClusterTopology, Value: "small"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := reconciler.buildRunnerPod(tc.test, tc.podName)
			if err != nil {
				t.Fatalf("buildRunnerPod() error: %v", err)
			}
			meta, err := startedMetadataJSON(tc.test)
			if err != nil {
				t.Fatalf("startedMetadataJSON() error: %v", err)
			}
			env := append(append([]corev1.EnvVar{}, tc.env...),
				corev1.EnvVar{Name: artifacts.EnvBucket, Value: reconciler.GCSBucket},
				corev1.EnvVar{Name: artifacts.EnvTestName, Value: tc.test.Name},
				corev1.EnvVar{Name: runner.EnvMetadata, Value: string(meta)},
				downwardEnv(artifacts.EnvPodNamespace, "metadata.namespace"),
				downwardEnv(artifacts.EnvPodUID, "metadata.uid"),
			)
			options, err := runnerOptionEnv(tc.test)
			if err != nil {
				t.Fatalf("runnerOptionEnv() error: %v", err)
			}
			env = append(env, options...)
			want := wantRunnerPod(tc.test, tc.podName, reconciler, env)
			if diff := cmp.Diff(want, got); diff != "" {
				t.Fatalf("buildRunnerPod() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStartedMetadataJSON(t *testing.T) {
	testCases := []struct {
		name string
		test *reliabilitytestv1alpha1.ReliabilityTest
		want runner.Metadata
	}{
		{
			name: "required fields",
			test: &reliabilitytestv1alpha1.ReliabilityTest{
				ObjectMeta: metav1.ObjectMeta{Name: "small", Namespace: "harness"},
				Spec: reliabilitytestv1alpha1.ReliabilityTestSpec{
					Scenario:      "reliability-small-cluster.yaml",
					Image:         "quay.io/example/reliability:latest",
					AuthSecretRef: corev1.LocalObjectReference{Name: "cluster-auth"},
				},
			},
			want: runner.Metadata{
				Name:       "small",
				Namespace:  "harness",
				PodName:    "small-runner",
				Scenario:   "reliability-small-cluster.yaml",
				Image:      "quay.io/example/reliability:latest",
				AuthSecret: "cluster-auth",
			},
		},
		{
			name: "start arguments preserved",
			test: &reliabilitytestv1alpha1.ReliabilityTest{
				ObjectMeta: metav1.ObjectMeta{Name: "full", Namespace: "harness"},
				Spec: reliabilitytestv1alpha1.ReliabilityTestSpec{
					Scenario:        "reliability.yaml",
					Image:           "quay.io/example/reliability:latest",
					AuthSecretRef:   corev1.LocalObjectReference{Name: "cluster-auth"},
					Duration:        "2d",
					ToleranceRate:   "5",
					FolderName:      "run1",
					Operators:       "netobserv-operator",
					Infra:           true,
					Upgrade:         true,
					ClusterTopology: "small",
					ImportDashboard: "dash.json",
				},
			},
			want: runner.Metadata{
				Name:            "full",
				Namespace:       "harness",
				PodName:         "full-runner",
				Scenario:        "reliability.yaml",
				Image:           "quay.io/example/reliability:latest",
				AuthSecret:      "cluster-auth",
				Duration:        "2d",
				ToleranceRate:   "5",
				FolderName:      "run1",
				Operators:       "netobserv-operator",
				Infra:           true,
				Upgrade:         true,
				ClusterTopology: "small",
				ImportDashboard: "dash.json",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := startedMetadataJSON(tc.test)
			if err != nil {
				t.Fatalf("startedMetadataJSON() error: %v", err)
			}
			var got runner.Metadata
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal started metadata: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("startedMetadataJSON() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func wantRunnerPod(test *reliabilitytestv1alpha1.ReliabilityTest, podName string, r *ReliabilityTestReconciler, env []corev1.EnvVar) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: test.Namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: new(false),
			Volumes: []corev1.Volume{
				{
					Name: "auth",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: test.Spec.AuthSecretRef.Name,
							Items: []corev1.KeyToPath{
								{Key: "kubeconfig", Path: "kubeconfig"},
								{Key: "admin", Path: "admin"},
								{Key: "users", Path: "users"},
							},
						},
					},
				},
				{
					Name: "gcs",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: r.GCSCredentialsSecret,
							Items: []corev1.KeyToPath{
								{Key: "service-account.json", Path: "service-account.json"},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:       "reliability",
					Image:      test.Spec.Image,
					WorkingDir: "/reliability-v2",
					Command:    []string{"/usr/local/bin/reliability-runner"},
					Env:        env,
					VolumeMounts: []corev1.VolumeMount{
						{Name: "auth", MountPath: "/auth", ReadOnly: true},
						{Name: "gcs", MountPath: "/gcs", ReadOnly: true},
					},
				},
			},
		},
	}
}
