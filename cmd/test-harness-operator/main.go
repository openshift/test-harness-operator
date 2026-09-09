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

	reliabilitytestv1alpha1 "github.com/droslean/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/droslean/test-harness-operator/pkg/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(reliabilitytestv1alpha1.AddToScheme(scheme))
}

type options struct {
	namespace string
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.namespace, "namespace", "", "Namespace the operator watches.")
	if err := fs.Parse(os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("failed to parse flags")
	}
	return o
}

func (o *options) Validate() error {
	if o.namespace == "" {
		return errors.New("required flag --namespace was unset")
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
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		logrus.WithError(err).Fatal("unable to create controller")
	}

	logrus.WithField("namespace", o.namespace).Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logrus.WithError(err).Fatal("problem running manager")
	}
}
