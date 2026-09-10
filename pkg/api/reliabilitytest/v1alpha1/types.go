package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	EnvKubeconfig      = "KUBECONFIG"
	EnvClusterTopology = "CLUSTER_TOPOLOGY"
	EnvImportDashboard = "IMPORT_DASHBOARD"
	EnvSlackMember     = "SLACK_MEMBER"
	EnvSlackChannel    = "SLACK_CHANNEL"
	EnvSlackAPIToken   = "SLACK_API_TOKEN"
	EnvSlackWebhookURL = "SLACK_WEBHOOK_URL"
)

// ReliabilityTestSpec defines the desired state of a reliability-v2 test run.
type ReliabilityTestSpec struct {
	// Scenario is the reliability-v2 config file name passed to start.sh -c,
	// e.g. reliability-small-cluster.yaml. The file must exist in the runner image.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Scenario string `json:"scenario"`

	// AuthSecretRef references a Secret in the same namespace that holds target-cluster auth
	// files: kubeconfig, admin, and users. Mounted at /auth and passed to start.sh as -p /auth.
	// +kubebuilder:validation:Required
	AuthSecretRef corev1.LocalObjectReference `json:"authSecretRef"`

	// Duration is how long the test should run, passed to start.sh -t
	// such as "10m", "2d", or "7d".
	// +kubebuilder:default="10m"
	// +optional
	Duration string `json:"duration,omitempty"`

	// Image is the container image that runs reliability-v2.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// ToleranceRate is passed to start.sh -r. Default in start.sh is 1.
	// +optional
	ToleranceRate string `json:"toleranceRate,omitempty"`

	// FolderName is passed to start.sh -n (output folder name).
	// +optional
	FolderName string `json:"folderName,omitempty"`

	// Infra enables start.sh -i (install infra nodes and move components).
	// +optional
	Infra bool `json:"infra,omitempty"`

	// Upgrade enables start.sh -u (upgrade the cluster every 24 hours).
	// +optional
	Upgrade bool `json:"upgrade,omitempty"`

	// Operators is a comma-separated list passed to start.sh -o.
	// +optional
	Operators string `json:"operators,omitempty"`

	// ClusterTopology is set as the CLUSTER_TOPOLOGY env var (logged by start.sh).
	// +optional
	ClusterTopology string `json:"clusterTopology,omitempty"`

	// ImportDashboard is set as the IMPORT_DASHBOARD env var for dittybopper.
	// +optional
	ImportDashboard string `json:"importDashboard,omitempty"`

	// Slack configures Slack-related env vars consumed by start.sh.
	// +optional
	Slack *SlackSpec `json:"slack,omitempty"`
}

// SlackSpec holds Slack settings that start.sh reads from the environment.
type SlackSpec struct {
	// APITokenSecretRef is the Secret key for SLACK_API_TOKEN.
	// +optional
	APITokenSecretRef *corev1.SecretKeySelector `json:"apiTokenSecretRef,omitempty"`

	// WebhookURLSecretRef is the Secret key for SLACK_WEBHOOK_URL.
	// +optional
	WebhookURLSecretRef *corev1.SecretKeySelector `json:"webhookURLSecretRef,omitempty"`

	// Member is set as SLACK_MEMBER.
	// +optional
	Member string `json:"member,omitempty"`

	// Channel is set as SLACK_CHANNEL.
	// +optional
	Channel string `json:"channel,omitempty"`
}

// ReliabilityTestStatus defines the observed state of a ReliabilityTest.
type ReliabilityTestStatus struct {
	// Phase is a high-level summary of the test run lifecycle.
	// +optional
	Phase string `json:"phase,omitempty"`

	// PodName is the name of the runner pod created for this test.
	// +optional
	PodName string `json:"podName,omitempty"`

	// Message provides additional detail about the current phase.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rtest
// +kubebuilder:printcolumn:name="Scenario",type=string,JSONPath=`.spec.scenario`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=`.status.podName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ReliabilityTest runs a reliability-v2 scenario as a pod in the cluster.
type ReliabilityTest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReliabilityTestSpec   `json:"spec,omitempty"`
	Status ReliabilityTestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ReliabilityTestList contains a list of ReliabilityTest.
type ReliabilityTestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReliabilityTest `json:"items"`
}

// StartShArgs returns arguments for ./start.sh given the auth mount path.
func (s ReliabilityTestSpec) StartShArgs(authPath string) []string {
	args := []string{"-p", authPath, "-c", s.Scenario}
	if s.Duration != "" {
		args = append(args, "-t", s.Duration)
	}
	if s.ToleranceRate != "" {
		args = append(args, "-r", s.ToleranceRate)
	}
	if s.FolderName != "" {
		args = append(args, "-n", s.FolderName)
	}
	if s.Operators != "" {
		args = append(args, "-o", s.Operators)
	}
	if s.Infra {
		args = append(args, "-i")
	}
	if s.Upgrade {
		args = append(args, "-u")
	}
	return args
}

// StartShEnv returns environment variables for ./start.sh given the auth mount path.
func (s ReliabilityTestSpec) StartShEnv(authPath string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: EnvKubeconfig, Value: authPath + "/kubeconfig"},
	}
	if s.ClusterTopology != "" {
		env = append(env, corev1.EnvVar{Name: EnvClusterTopology, Value: s.ClusterTopology})
	}
	if s.ImportDashboard != "" {
		env = append(env, corev1.EnvVar{Name: EnvImportDashboard, Value: s.ImportDashboard})
	}
	if s.Slack == nil {
		return env
	}
	if s.Slack.Member != "" {
		env = append(env, corev1.EnvVar{Name: EnvSlackMember, Value: s.Slack.Member})
	}
	if s.Slack.Channel != "" {
		env = append(env, corev1.EnvVar{Name: EnvSlackChannel, Value: s.Slack.Channel})
	}
	if s.Slack.APITokenSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: EnvSlackAPIToken,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: s.Slack.APITokenSecretRef,
			},
		})
	}
	if s.Slack.WebhookURLSecretRef != nil {
		env = append(env, corev1.EnvVar{
			Name: EnvSlackWebhookURL,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: s.Slack.WebhookURLSecretRef,
			},
		})
	}
	return env
}

func init() {
	SchemeBuilder.Register(&ReliabilityTest{}, &ReliabilityTestList{})
}
