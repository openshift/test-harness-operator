package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	reliabilitytestv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/openshift/test-harness-operator/pkg/artifacts"
	"github.com/openshift/test-harness-operator/pkg/runner"
)

const (
	runnerPodSuffix = "runner"
	authMountPath   = "/auth"
	authVolume      = "auth"
	gcsVolume       = "gcs"
)

// RunnerPodName returns the runner pod name for a ReliabilityTest.
func RunnerPodName(testName string) string {
	return fmt.Sprintf("%s-%s", testName, runnerPodSuffix)
}

// ReliabilityTestReconciler reconciles a ReliabilityTest object.
type ReliabilityTestReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	GCSBucket            string
	GCSCredentialsSecret string
}

// +kubebuilder:rbac:groups=harness.testharness.io,resources=reliabilitytests,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=harness.testharness.io,resources=reliabilitytests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=harness.testharness.io,resources=reliabilitytests/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *ReliabilityTestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	test := &reliabilitytestv1alpha1.ReliabilityTest{}
	if err := r.Get(ctx, req.NamespacedName, test); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	podName := RunnerPodName(test.Name)
	pod := &corev1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Name: podName, Namespace: test.Namespace}, pod)
	if apierrors.IsNotFound(err) {
		pod, err = r.buildRunnerPod(test, podName)
		if err != nil {
			logrus.WithError(err).Error("failed to build runner pod")
			return ctrl.Result{}, err
		}
		if err := controllerutil.SetControllerReference(test, pod, r.Scheme); err != nil {
			logrus.WithError(err).Error("failed to set controller reference on runner pod")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, pod); err != nil {
			logrus.WithError(err).WithField("pod", podName).Error("failed to create runner pod")
			return ctrl.Result{}, err
		}
		logrus.WithField("pod", podName).WithField("scenario", test.Spec.Scenario).Info("created reliability runner pod")
		return r.updateStatus(ctx, test, string(corev1.PodPending), podName, "")
	}
	if err != nil {
		logrus.WithError(err).WithField("pod", podName).Error("failed to get runner pod")
		return ctrl.Result{}, err
	}

	return r.updateStatus(ctx, test, string(pod.Status.Phase), pod.Name, pod.Status.Message)
}

func (r *ReliabilityTestReconciler) buildRunnerPod(test *reliabilitytestv1alpha1.ReliabilityTest, podName string) (*corev1.Pod, error) {
	meta, err := startedMetadataJSON(test)
	if err != nil {
		return nil, err
	}
	env := append(test.Spec.StartShEnv(authMountPath),
		corev1.EnvVar{Name: artifacts.EnvBucket, Value: r.GCSBucket},
		corev1.EnvVar{Name: artifacts.EnvTestName, Value: test.Name},
		corev1.EnvVar{Name: runner.EnvMetadata, Value: string(meta)},
		downwardEnv(artifacts.EnvPodNamespace, "metadata.namespace"),
		downwardEnv(artifacts.EnvPodUID, "metadata.uid"),
	)
	options, err := runnerOptionEnv(test)
	if err != nil {
		return nil, err
	}
	env = append(env, options...)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: test.Namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: new(false),
			Volumes:                      runnerVolumes(test.Spec.AuthSecretRef.Name, r.GCSCredentialsSecret),
			Containers:                   []corev1.Container{testContainer(test, env)},
		},
	}, nil
}

func startedMetadataJSON(test *reliabilitytestv1alpha1.ReliabilityTest) ([]byte, error) {
	return json.Marshal(runner.Metadata{
		Name:            test.Name,
		Namespace:       test.Namespace,
		PodName:         RunnerPodName(test.Name),
		Scenario:        test.Spec.Scenario,
		Image:           test.Spec.Image,
		AuthSecret:      test.Spec.AuthSecretRef.Name,
		Duration:        test.Spec.Duration,
		ToleranceRate:   test.Spec.ToleranceRate,
		FolderName:      test.Spec.FolderName,
		Operators:       test.Spec.Operators,
		Infra:           test.Spec.Infra,
		Upgrade:         test.Spec.Upgrade,
		ClusterTopology: test.Spec.ClusterTopology,
		ImportDashboard: test.Spec.ImportDashboard,
	})
}

func runnerVolumes(authSecret, gcsSecret string) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: authVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: authSecret,
					Items: []corev1.KeyToPath{
						{Key: "kubeconfig", Path: "kubeconfig"},
						{Key: "admin", Path: "admin"},
						{Key: "users", Path: "users"},
					},
				},
			},
		},
		{
			Name: gcsVolume,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: gcsSecret,
					Items: []corev1.KeyToPath{
						{Key: artifacts.CredentialsKey, Path: artifacts.CredentialsKey},
					},
				},
			},
		},
	}
}

func testContainer(test *reliabilitytestv1alpha1.ReliabilityTest, env []corev1.EnvVar) corev1.Container {
	return corev1.Container{
		Name:       artifacts.TestContainer,
		Image:      test.Spec.Image,
		WorkingDir: artifacts.SuiteDir,
		Command:    []string{artifacts.RunnerBinary},
		Env:        env,
		VolumeMounts: []corev1.VolumeMount{
			{Name: authVolume, MountPath: authMountPath, ReadOnly: true},
			{Name: gcsVolume, MountPath: artifacts.CredentialsMount, ReadOnly: true},
		},
	}
}

func runnerOptionEnv(test *reliabilitytestv1alpha1.ReliabilityTest) ([]corev1.EnvVar, error) {
	raw, err := json.Marshal(runner.StartOptions{
		AuthPath:      authMountPath,
		Scenario:      test.Spec.Scenario,
		Duration:      test.Spec.Duration,
		ToleranceRate: test.Spec.ToleranceRate,
		FolderName:    test.Spec.FolderName,
		Operators:     test.Spec.Operators,
		Infra:         test.Spec.Infra,
		Upgrade:       test.Spec.Upgrade,
	})
	if err != nil {
		return nil, err
	}
	return []corev1.EnvVar{{Name: runner.EnvOptions, Value: string(raw)}}, nil
}

func downwardEnv(name, fieldPath string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath},
		},
	}
}

func (r *ReliabilityTestReconciler) updateStatus(ctx context.Context, test *reliabilitytestv1alpha1.ReliabilityTest, phase, podName, message string) (ctrl.Result, error) {
	if test.Status.Phase == phase && test.Status.PodName == podName && test.Status.Message == message {
		return ctrl.Result{}, nil
	}

	updated := test.DeepCopy()
	updated.Status.Phase = phase
	updated.Status.PodName = podName
	updated.Status.Message = message

	if err := r.Status().Update(ctx, updated); err != nil {
		logrus.WithError(err).WithField("name", test.Name).Error("failed to update ReliabilityTest status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ReliabilityTestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&reliabilitytestv1alpha1.ReliabilityTest{}).Owns(&corev1.Pod{}).Named("reliabilitytest").Complete(r)
}
