package cluster

import (
	"path"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
)

const (
	finalizer    = "cluster.harness.testharness.io/destroy"
	clusterLabel = "harness.testharness.io/cluster"

	installDir            = "/install"
	credsDir              = "/credentials"
	installConfigFilename = "install-config.yaml"
)

func installConfigName(clusterName string) string {
	return clusterName + "-install-config"
}

func createJobName(clusterName string) string {
	return clusterName + "-create"
}

func destroyJobName(clusterName string) string {
	return clusterName + "-destroy"
}

func clusterLabels(clusterName string) map[string]string {
	return map[string]string{clusterLabel: clusterName}
}

func createJob(cluster *clusterv1alpha1.Cluster) *batchv1.Job {
	container := installerContainer(cluster, "create")
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "install-config",
		MountPath: path.Join(installDir, installConfigFilename),
		SubPath:   installConfigFilename,
		ReadOnly:  true,
	})
	return &batchv1.Job{
		ObjectMeta: jobObjectMeta(cluster, createJobName(cluster.Name)),
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(0)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: clusterLabels(cluster.Name)},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: new(false),
					Containers:                   []corev1.Container{container},
					Volumes: append(installerVolumes(cluster), corev1.Volume{
						Name: "install-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: installConfigName(cluster.Name)},
							},
						},
					}),
				},
			},
		},
	}
}

func destroyJob(cluster *clusterv1alpha1.Cluster) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: jobObjectMeta(cluster, destroyJobName(cluster.Name)),
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(5)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: clusterLabels(cluster.Name)},
				Spec: corev1.PodSpec{
					RestartPolicy:                corev1.RestartPolicyNever,
					AutomountServiceAccountToken: new(false),
					Containers:                   []corev1.Container{installerContainer(cluster, "destroy")},
					Volumes:                      installerVolumes(cluster),
				},
			},
		},
	}
}

func jobObjectMeta(cluster *clusterv1alpha1.Cluster, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: cluster.Namespace,
		Labels:    clusterLabels(cluster.Name),
	}
}

func installerContainer(cluster *clusterv1alpha1.Cluster, action string) corev1.Container {
	return corev1.Container{
		Name:    "installer",
		Image:   cluster.Spec.InstallerImage,
		Command: []string{"openshift-install", action, "cluster", "--dir", installDir},
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: cluster.Spec.CredentialsSecretRef,
			},
		}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: "install", MountPath: installDir},
			{Name: "credentials", MountPath: credsDir, ReadOnly: true},
		},
	}
}

func installerVolumes(cluster *clusterv1alpha1.Cluster) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: "install",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: cluster.Spec.CredentialsSecretRef.Name,
				},
			},
		},
	}
}
