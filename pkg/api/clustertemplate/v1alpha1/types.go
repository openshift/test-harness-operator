package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=hct
// +kubebuilder:printcolumn:name="Installer",type=string,JSONPath=`.spec.installerImage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterTemplate is a saved Cluster spec.
// Creating a Cluster copies this spec. The template is not linked to clusters created from it.
type ClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec clusterv1alpha1.ClusterSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterTemplateList contains a list of ClusterTemplate.
type ClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterTemplate{}, &ClusterTemplateList{})
}
