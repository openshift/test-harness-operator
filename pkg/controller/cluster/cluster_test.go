package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	installertypes "github.com/openshift/installer/pkg/types"
	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
)

var scheme = runtime.NewScheme()

func init() {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := clusterv1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
}

func TestReconcile(t *testing.T) {
	testCases := []struct {
		name    string
		objects []client.Object
		jobs    []batchv1.Job
	}{
		{
			name: "missing cluster",
		},
		{
			name: "missing credentials secret fails the cluster",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
				},
			},
		},
		{
			name: "create starts an install job",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "aws-lrc", Namespace: "harness"},
					Data:       map[string][]byte{"credentials": []byte("[default]\n")},
				},
			},
			jobs: []batchv1.Job{{
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
			}},
		},
		{
			name: "completed install is ready",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
				},
				&batchv1.Job{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
				},
			},
			jobs: []batchv1.Job{{
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
				Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
			}},
		},
		{
			name: "failed install stays failed",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "edge", Namespace: "harness"},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
				},
				&batchv1.Job{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}},
				},
			},
			jobs: []batchv1.Job{{
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
				Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}}},
			}},
		},
		{
			name: "delete before install drops the finalizer",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "edge",
						Namespace:         "harness",
						Finalizers:        []string{finalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)},
					},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
				},
			},
		},
		{
			name: "delete stops a running install",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "edge",
						Namespace:         "harness",
						Finalizers:        []string{finalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)},
					},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
					Status: clusterv1alpha1.ClusterStatus{Phase: clusterv1alpha1.ClusterPhaseProvisioning},
				},
				&batchv1.Job{
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
		},
		{
			name: "delete of a ready cluster starts destroy",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "edge",
						Namespace:         "harness",
						Finalizers:        []string{finalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)},
					},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
					Status: clusterv1alpha1.ClusterStatus{Phase: clusterv1alpha1.ClusterPhaseReady},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "aws-lrc", Namespace: "harness"},
					Data:       map[string][]byte{"credentials": []byte("[default]\n")},
				},
				&batchv1.Job{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
				},
			},
			jobs: []batchv1.Job{
				{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
				},
				{
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
		},
		{
			name: "completed destroy removes the cluster finalizer",
			objects: []client.Object{
				&clusterv1alpha1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "edge",
						Namespace:         "harness",
						Finalizers:        []string{finalizer},
						DeletionTimestamp: &metav1.Time{Time: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)},
					},
					Spec: clusterv1alpha1.ClusterSpec{
						InstallerImage:       "quay.io/example/installer:4.12",
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "aws-lrc"},
						InstallConfig: installertypes.InstallConfig{
							ObjectMeta: metav1.ObjectMeta{Name: "edge"},
							BaseDomain: "example.com",
						},
					},
					Status: clusterv1alpha1.ClusterStatus{Phase: clusterv1alpha1.ClusterPhaseReady},
				},
				&batchv1.Job{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
				},
				&batchv1.Job{
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
					Status: batchv1.JobStatus{Conditions: []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&clusterv1alpha1.Cluster{}, &batchv1.Job{}).
				WithObjects(tc.objects...).
				Build()

			reconciler := &ClusterReconciler{Client: c, Scheme: scheme}
			if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "edge", Namespace: "harness"},
			}); err != nil {
				t.Fatalf("Reconcile() error: %v", err)
			}

			var got batchv1.JobList
			if err := c.List(context.Background(), &got, client.InNamespace("harness")); err != nil {
				t.Fatalf("List jobs: %v", err)
			}

			if diff := cmp.Diff(tc.jobs, got.Items, cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"), cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("jobs mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
