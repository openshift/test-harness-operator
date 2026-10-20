package v1alpha1

import (
	installertypes "github.com/openshift/installer/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterProfileTemplateSpec is the install configuration stored on a template.
// A Cluster created from this template copies these fields onto its own spec.
type ClusterProfileTemplateSpec struct {
	// InstallerImage is the container image that provides openshift-install.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	InstallerImage string `json:"installerImage"`

	// InstallConfig is the OpenShift install configuration for this template.
	// +kubebuilder:validation:Required
	InstallConfig installertypes.InstallConfig `json:"installConfig"`

	// CredentialsSecretRef references a Secret in the same namespace that holds
	// the cloud credential files the installer reads from disk.
	// +kubebuilder:validation:Required
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=hcpt
// +kubebuilder:printcolumn:name="Installer",type=string,JSONPath=`.spec.installerImage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterProfileTemplate is a catalog entry for an OpenShift install configuration.
// Creating a Cluster copies this spec. The template is not linked to clusters created from it.
type ClusterProfileTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ClusterProfileTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterProfileTemplateList contains a list of ClusterProfileTemplate.
type ClusterProfileTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProfileTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterProfileTemplate{}, &ClusterProfileTemplateList{})
}
