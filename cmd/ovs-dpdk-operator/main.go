/*
 * Copyright 2026 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	operatorv1alpha "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/api/ovsdpdkoperator/v1alpha1"

	"github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/consts"
	"github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/flags"

	controllers "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/controllers/operator"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(operatorv1alpha.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	if err := newApp().Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newApp() *cli.App {
	var draDriverImage string
	var draDriverNamespace string
	var operatorNamespace string
	var kubeClientConfig flags.KubeClientConfig
	loggingConfig := flags.NewLoggingConfig()

	cliFlags := []cli.Flag{
		&cli.StringFlag{
			Name:        "dra-driver-ovsdpdk-image",
			Usage:       "OVSDPDK DRA driver container image reference",
			Required:    true,
			Destination: &draDriverImage,
			EnvVars:     []string{"DRA_DRIVER_IMAGE"},
		},
		&cli.StringFlag{
			Name:        "dra-driver-ovsdpdk-namespace",
			Usage:       "OVSDPDK DRA driver container image reference",
			Value:       consts.DefaultNamespace,
			Required:    true,
			Destination: &draDriverNamespace,
			EnvVars:     []string{"DRA_DRIVER_NAMESPACE"},
		},
		&cli.StringFlag{
			Name:        "namespace",
			Usage:       "Namespace tracked by the operator for custom resources",
			Value:       consts.DefaultOperatorNamespace,
			Destination: &operatorNamespace,
			EnvVars:     []string{"NAMESPACE"},
		},
	}

	cliFlags = append(cliFlags, kubeClientConfig.Flags()...)
	cliFlags = append(cliFlags, loggingConfig.Flags()...)

	app := &cli.App{
		Name:            "ovs-dpdk-operator",
		Usage:           "ovs-dpdk-operator is a kubernetes operator that manages the lifecycle of OVS DPDK bridges and ports on a cluster",
		ArgsUsage:       " ",
		HideHelpCommand: true,
		Flags:           cliFlags,
		Before: func(c *cli.Context) error {
			if err := loggingConfig.Apply(); err != nil {
				return err
			}
			// Wire controller-runtime to use the same klog backend so its
			// internal logs are not silently dropped.
			ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))
			return nil
		},
		Action: func(c *cli.Context) error {
			restCfg, err := kubeClientConfig.RestConfig()
			if err != nil {
				return fmt.Errorf("create REST config: %w", err)
			}

			k8sClient, err := kubeClientConfig.NewCoreClient()
			if err != nil {
				return fmt.Errorf("create client: %w", err)
			}

			mgr, err := newManager(restCfg, k8sClient, operatorNamespace)
			if err != nil {
				return err
			}

			return runOperator(c.Context, mgr, operatorNamespace, draDriverImage, draDriverNamespace)
		},
	}

	return app
}

func newManager(restCfg *rest.Config, k8sClient coreclientset.Interface, operatorNamespace string) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(
		restCfg,
		ctrl.Options{
			Scheme: scheme,
			Metrics: metricsserver.Options{
				BindAddress: "0", // disabled
			},
			HealthProbeBindAddress: "0", // disabled
			LeaderElection:         false,
			Cache: cache.Options{
				DefaultNamespaces: map[string]cache.Config{
					operatorNamespace: {},
				},
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create controller manager: %w", err)
	}

	return mgr, nil
}

func runOperator(
	ctx context.Context,
	manager ctrl.Manager,
	operatorNamespace string,
	draDriverContainerImage string,
	draDriverNamespace string,
) error {
	logger := klog.FromContext(ctx).WithName("main")

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	ctx, cancelMainContext := context.WithCancelCause(ctx)

	logger.Info("Starting ovs-dpdk-operator",
		"namespace", operatorNamespace,
		"draDriverImage", draDriverContainerImage,
		"draDriverNamespace", draDriverNamespace,
	)

	operatorConfigReconciler := controllers.NewOvsDpdkOperatorConfigReconciler(manager.GetClient())
	err := operatorConfigReconciler.SetupWithManager(manager)
	if err != nil {
		return fmt.Errorf("operator configuration controller could not be configured: %w", err)
	}

	managerErrorChannel := make(chan error, 1)
	go func() {
		if err := manager.Start(ctx); err != nil {
			managerErrorChannel <- fmt.Errorf("controller manager exited: %w", err)
		}
		close(managerErrorChannel)
	}()

	select {
	case <-ctx.Done():
	case err := <-managerErrorChannel:
		if err != nil {
			cancelMainContext(err)
		}
	}

	stop()

	if err := context.Cause(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error(err, "Shutting down ovs-dpdk-operator due to error")
		return err
	}
	logger.V(1).Info("Shutting down ovs-dpdk-operator cleanly")
	return nil
}
