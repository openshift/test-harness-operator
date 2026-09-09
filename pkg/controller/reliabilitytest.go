package controller

import (
	"context"
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

	reliabilitytestv1alpha1 "github.com/droslean/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
)

const (
	runnerPodSuffix = "runner"
	authMountPath   = "/auth"
)

// RunnerPodName returns the runner pod name for a ReliabilityTest.
func RunnerPodName(testName string) string {
	return fmt.Sprintf("%s-%s", testName, runnerPodSuffix)
}

// ReliabilityTestReconciler reconciles a ReliabilityTest object.
type ReliabilityTestReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
		pod = r.buildRunnerPod(test, podName)
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

func (r *ReliabilityTestReconciler) buildRunnerPod(test *reliabilitytestv1alpha1.ReliabilityTest, podName string) *corev1.Pod {
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
			},
			Containers: []corev1.Container{
				{
					Name:       "reliability",
					Image:      test.Spec.Image,
					WorkingDir: "/reliability-v2",
					Command:    []string{"./start.sh"},
					Args:       test.Spec.StartShArgs(authMountPath),
					Env:        test.Spec.StartShEnv(authMountPath),
					VolumeMounts: []corev1.VolumeMount{
						{Name: "auth", MountPath: authMountPath, ReadOnly: true},
					},
				},
			},
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
