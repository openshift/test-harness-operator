package main

import (
	"errors"
	"flag"
	"os"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	reliabilitytestv1alpha1 "github.com/openshift/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/openshift/test-harness-operator/pkg/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(reliabilitytestv1alpha1.AddToScheme(scheme))
}

type options struct {
	namespace            string
	gcsBucket            string
	gcsCredentialsSecret string
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.namespace, "namespace", "", "Namespace the operator watches.")
	fs.StringVar(&o.gcsBucket, "gcs-bucket", "", "GCS bucket for test artifacts.")
	fs.StringVar(&o.gcsCredentialsSecret, "gcs-credentials-secret", "", "Secret in the watched namespace containing the GCS service account key service-account.json.")
	if err := fs.Parse(os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("failed to parse flags")
	}
	return o
}

func (o *options) Validate() error {
	if o.namespace == "" {
		return errors.New("required flag --namespace was unset")
	}
	if o.gcsBucket == "" {
		return errors.New("required flag --gcs-bucket was unset")
	}
	if o.gcsCredentialsSecret == "" {
		return errors.New("required flag --gcs-credentials-secret was unset")
	}
	return nil
}

func main() {
	ctrl.SetLogger(zap.New())

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("invalid options")
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		logrus.WithError(err).Fatal("unable to load kubeconfig")
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				o.namespace: {},
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Fatal("unable to start manager")
	}

	if err = (&controller.ReliabilityTestReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		GCSBucket:            o.gcsBucket,
		GCSCredentialsSecret: o.gcsCredentialsSecret,
	}).SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller")
	}

	logrus.WithField("namespace", o.namespace).Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("problem running manager")
	}
}
