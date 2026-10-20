package cluster

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
)

func TestCreateJob(t *testing.T) {
	testCases := []struct {
		name    string
		cluster *clusterv1alpha1.Cluster
		want    *batchv1.Job
	}{
		{
			name: "install job",
			cluster: &clusterv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
				Spec: clusterv1alpha1.ClusterSpec{
					InstallerImage:       "quay.io/example/installer:4.12",
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
				},
			},
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-create",
					Namespace: "harness",
					Labels:    map[string]string{"harness.testharness.io/cluster": "edge"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr.To(int32(0)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"harness.testharness.io/cluster": "edge"}},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: new(false),
							Containers: []corev1.Container{{
								Name:    "installer",
								Image:   "quay.io/example/installer:4.12",
								Command: []string{"openshift-install", "create", "cluster", "--dir", "/install"},
								EnvFrom: []corev1.EnvFromSource{{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "aws-lrc"},
									},
								}},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "install", MountPath: "/install"},
									{Name: "credentials", MountPath: "/credentials", ReadOnly: true},
									{Name: "install-config", MountPath: "/install/install-config.yaml", SubPath: "install-config.yaml", ReadOnly: true},
								},
							}},
							Volumes: []corev1.Volume{
								{Name: "install", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
								{Name: "credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "aws-lrc"}}},
								{Name: "install-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "edge-install-config"}}}},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, createJob(tc.cluster)); diff != "" {
				t.Fatalf("createJob() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDestroyJob(t *testing.T) {
	testCases := []struct {
		name    string
		cluster *clusterv1alpha1.Cluster
		want    *batchv1.Job
	}{
		{
			name: "destroy job",
			cluster: &clusterv1alpha1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
				Spec: clusterv1alpha1.ClusterSpec{
					InstallerImage:       "quay.io/example/installer:4.12",
					CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
				},
			},
			want: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge-destroy",
					Namespace: "harness",
					Labels:    map[string]string{"harness.testharness.io/cluster": "edge"},
				},
				Spec: batchv1.JobSpec{
					BackoffLimit: ptr.To(int32(5)),
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"harness.testharness.io/cluster": "edge"}},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyNever,
							AutomountServiceAccountToken: new(false),
							Containers: []corev1.Container{{
								Name:    "installer",
								Image:   "quay.io/example/installer:4.12",
								Command: []string{"openshift-install", "destroy", "cluster", "--dir", "/install"},
								EnvFrom: []corev1.EnvFromSource{{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "aws-lrc"},
									},
								}},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "install", MountPath: "/install"},
									{Name: "credentials", MountPath: "/credentials", ReadOnly: true},
								},
							}},
							Volumes: []corev1.Volume{
								{Name: "install", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
								{Name: "credentials", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "aws-lrc"}}},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, destroyJob(tc.cluster)); diff != "" {
				t.Fatalf("destroyJob() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
