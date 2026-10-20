package cluster

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	installertypes "github.com/openshift/installer/pkg/types"
	clusterv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/cluster/v1alpha1"
)

// ClusterReconciler reconciles a Cluster object.
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=harness.testharness.io,resources=clusters,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=harness.testharness.io,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=harness.testharness.io,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete

// Reconcile creates a cluster with openshift-install and destroys it when the Cluster is deleted.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	cluster := &clusterv1alpha1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}
	return r.reconcileCreate(ctx, cluster)
}

func (r *ClusterReconciler) reconcileCreate(ctx context.Context, cluster *clusterv1alpha1.Cluster) (ctrl.Result, error) {
	if controllerutil.AddFinalizer(cluster, finalizer) {
		if err := r.Update(ctx, cluster); err != nil {
			logrus.WithError(err).WithField("cluster", cluster.Name).Error("failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	job, err := r.getJob(ctx, cluster.Namespace, createJobName(cluster.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	if job == nil {
		if err := r.credentials(ctx, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseFailed, fmt.Sprintf("credentials secret %s not found", cluster.Spec.CredentialsSecretRef.Name))
			}
			return ctrl.Result{}, err
		}
		cfg := cluster.Spec.InstallConfig
		if cfg.APIVersion == "" {
			cfg.APIVersion = installertypes.InstallConfigVersion
		}
		raw, err := yaml.Marshal(cfg)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("render install-config: %w", err)
		}
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      installConfigName(cluster.Name),
				Namespace: cluster.Namespace,
				Labels:    clusterLabels(cluster.Name),
			},
			Data: map[string]string{
				installConfigFilename: string(raw),
			},
		}
		existing := &corev1.ConfigMap{}
		err = r.Get(ctx, client.ObjectKeyFromObject(configMap), existing)
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, configMap); err != nil && !apierrors.IsAlreadyExists(err) {
				logrus.WithError(err).WithField("configmap", configMap.Name).Error("failed to create install-config configmap")
				return ctrl.Result{}, err
			}
		} else if err != nil {
			return ctrl.Result{}, err
		} else {
			existing.Data = configMap.Data
			existing.Labels = configMap.Labels
			if err := r.Update(ctx, existing); err != nil {
				return ctrl.Result{}, err
			}
		}
		job = createJob(cluster)
		if err := r.Create(ctx, job); err != nil {
			logrus.WithError(err).WithField("job", job.Name).Error("failed to create install job")
			return ctrl.Result{}, err
		}
		logrus.WithField("cluster", cluster.Name).Info("created cluster install job")
		return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseProvisioning, "install job created")
	}

	condition, finished := jobFinished(job.Status.Conditions)
	if !finished {
		return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseProvisioning, "install job running")
	}
	if condition.Type == batchv1.JobComplete {
		return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseReady, "cluster created")
	}
	return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseFailed, conditionMessage(condition, "install job failed"))
}

func (r *ClusterReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1alpha1.Cluster) (ctrl.Result, error) {
	create, err := r.getJob(ctx, cluster.Namespace, createJobName(cluster.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	if create != nil {
		if _, finished := jobFinished(create.Status.Conditions); !finished {
			if err := r.Delete(ctx, create); err != nil && !apierrors.IsNotFound(err) {
				logrus.WithError(err).WithField("job", create.Name).Error("failed to delete install job")
				return ctrl.Result{}, err
			}
			return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseDeleting, "stopping install job")
		}
	}

	if create == nil {
		switch cluster.Status.Phase {
		case clusterv1alpha1.ClusterPhaseProvisioning,
			clusterv1alpha1.ClusterPhaseReady,
			clusterv1alpha1.ClusterPhaseFailed,
			clusterv1alpha1.ClusterPhaseDeleting,
			clusterv1alpha1.ClusterPhaseDeleteFailed:
		default:
			return r.finishDelete(ctx, cluster)
		}
	}

	job, err := r.getJob(ctx, cluster.Namespace, destroyJobName(cluster.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	if job == nil {
		if err := r.credentials(ctx, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseDeleteFailed, fmt.Sprintf("credentials secret %s not found", cluster.Spec.CredentialsSecretRef.Name))
			}
			return ctrl.Result{}, err
		}
		job = destroyJob(cluster)
		if err := r.Create(ctx, job); err != nil {
			logrus.WithError(err).WithField("job", job.Name).Error("failed to create destroy job")
			return ctrl.Result{}, err
		}
		logrus.WithField("cluster", cluster.Name).Info("created cluster destroy job")
		return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseDeleting, "destroy job created")
	}

	condition, finished := jobFinished(job.Status.Conditions)
	if !finished {
		return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseDeleting, "destroy job running")
	}
	if condition.Type == batchv1.JobComplete {
		return r.finishDelete(ctx, cluster)
	}
	return r.updateStatus(ctx, cluster, clusterv1alpha1.ClusterPhaseDeleteFailed, conditionMessage(condition, "destroy job failed"))
}

func jobFinished(conditions []batchv1.JobCondition) (batchv1.JobCondition, bool) {
	for _, condition := range conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete, batchv1.JobFailed:
			return condition, true
		}
	}
	return batchv1.JobCondition{}, false
}

func conditionMessage(condition batchv1.JobCondition, fallback string) string {
	if condition.Message == "" {
		return fallback
	}
	return condition.Message
}

func (r *ClusterReconciler) finishDelete(ctx context.Context, cluster *clusterv1alpha1.Cluster) (ctrl.Result, error) {
	for _, name := range []string{createJobName(cluster.Name), destroyJobName(cluster.Name)} {
		if err := r.deleteIfPresent(ctx, &batchv1.Job{ObjectMeta: objectMeta(name, cluster.Namespace)}); err != nil {
			return ctrl.Result{}, err
		}
	}
	if err := r.deleteIfPresent(ctx, &corev1.ConfigMap{ObjectMeta: objectMeta(installConfigName(cluster.Name), cluster.Namespace)}); err != nil {
		return ctrl.Result{}, err
	}
	if controllerutil.RemoveFinalizer(cluster, finalizer) {
		if err := r.Update(ctx, cluster); err != nil {
			logrus.WithError(err).WithField("cluster", cluster.Name).Error("failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) credentials(ctx context.Context, cluster *clusterv1alpha1.Cluster) error {
	secret := &corev1.Secret{}
	name := types.NamespacedName{Name: cluster.Spec.CredentialsSecretRef.Name, Namespace: cluster.Namespace}
	return r.Get(ctx, name, secret)
}

func (r *ClusterReconciler) getJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, job)
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (r *ClusterReconciler) deleteIfPresent(ctx context.Context, obj client.Object) error {
	if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
		logrus.WithError(err).WithField("name", obj.GetName()).Error("failed to delete object")
		return err
	}
	return nil
}

func (r *ClusterReconciler) updateStatus(ctx context.Context, cluster *clusterv1alpha1.Cluster, phase, message string) (ctrl.Result, error) {
	if cluster.Status.Phase == phase && cluster.Status.Message == message {
		return ctrl.Result{}, nil
	}
	updated := cluster.DeepCopy()
	updated.Status.Phase = phase
	updated.Status.Message = message
	if err := r.Status().Update(ctx, updated); err != nil {
		logrus.WithError(err).WithField("cluster", cluster.Name).Error("failed to update Cluster status")
		return ctrl.Result{}, err
	}
	cluster.Status = updated.Status
	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.Cluster{}).
		Watches(&batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			name := obj.GetLabels()[clusterLabel]
			if name == "" {
				return nil
			}
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: name, Namespace: obj.GetNamespace()},
			}}
		})).
		Named("cluster").
		Complete(r)
}

func objectMeta(name, namespace string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace}
}
