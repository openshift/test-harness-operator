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
	"sigs.k8s.io/controller-runtime/pkg/client"

	reliabilitytestv1alpha1 "github.com/droslean/test-harness-operator/pkg/api/reliabilitytest/v1alpha1"
	"github.com/droslean/test-harness-operator/pkg/ui"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(reliabilitytestv1alpha1.AddToScheme(scheme))
}

type options struct {
	namespace string
	addr      string
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.namespace, "namespace", "", "Namespace the UI manages.")
	fs.StringVar(&o.addr, "addr", ":8090", "Address the UI listens on.")
	if err := fs.Parse(os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("failed to parse flags")
	}
	return o
}

func (o *options) Validate() error {
	if o.namespace == "" {
		return errors.New("required flag --namespace was unset")
	}
	if o.addr == "" {
		return errors.New("required flag --addr was unset")
	}
	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("invalid options")
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		logrus.WithError(err).Fatal("unable to load kubeconfig")
	}

	c, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logrus.WithError(err).Fatal("unable to create client")
	}

	server, err := ui.New(cfg, c, o.namespace)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create UI server")
	}

	logrus.WithFields(logrus.Fields{
		"namespace": o.namespace,
		"addr":      o.addr,
	}).Info("starting UI")
	if err := server.Start(ctrl.SetupSignalHandler(), o.addr); err != nil {
		logrus.WithError(err).Fatal("UI server failed")
	}
}
