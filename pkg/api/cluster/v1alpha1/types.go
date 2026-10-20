package v1alpha1

import (
	installertypes "github.com/openshift/installer/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ClusterPhasePending      = "Pending"
	ClusterPhaseProvisioning = "Provisioning"
	ClusterPhaseReady        = "Ready"
	ClusterPhaseFailed       = "Failed"
	ClusterPhaseDeleting     = "Deleting"
	ClusterPhaseDeleteFailed = "DeleteFailed"
)

// ClusterSpec is the install configuration for one cluster.
// It is a copy of a ClusterProfileTemplate spec and does not reference that template.
type ClusterSpec struct {
	// InstallerImage is the container image that provides openshift-install.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	InstallerImage string `json:"installerImage"`

	// InstallConfig is the OpenShift install configuration for this cluster.
	// +kubebuilder:validation:Required
	InstallConfig installertypes.InstallConfig `json:"installConfig"`

	// CredentialsSecretRef references a Secret in the same namespace that holds
	// the cloud credential files the installer reads from disk.
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
}

// ClusterStatus is the observed state of a Cluster.
type ClusterStatus struct {
	// Phase is a high-level summary of the cluster lifecycle.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Message provides additional detail about the current phase.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=hcluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Cluster is a self-contained OpenShift install request.
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster.
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
