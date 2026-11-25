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

package operator

import (
	"bytes"
	"context"
	"fmt"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovsdpdkoperatorv1alpha1 "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/api/ovsdpdkoperator/v1alpha1"
	"github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/consts"
	"github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/dependents"
)

type OvsDpdkOperatorConfigReconciler struct {
	client.Client
	log        klog.Logger
	daemonSets []client.ObjectKey
}

func NewOvsDpdkOperatorConfigReconciler(
	c client.Client,
) *OvsDpdkOperatorConfigReconciler {
	return &OvsDpdkOperatorConfigReconciler{
		Client:     c,
		log:        klog.Background().WithName("OvsDpdkOperatorConfigReconciler"),
		daemonSets: make([]client.ObjectKey, 1),
	}
}

// Core resources: services, endpoints, events, configmaps need full CRUD for handler deployment manifests.
// +kubebuilder:rbac:groups="",resources=services;endpoints;events,verbs=get;list;watch;create;update;patch;delete
// ServiceAccounts: operator creates dra-driver SA.
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// Namespaces: operator creates dra-driver namespace
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
// RBAC: operator creates handler ClusterRoles/Roles and bindings.
// escalate is required because the handler ClusterRole contains permissions the operator itself does not hold.
// bind is required on roles/clusterroles to create bindings referencing those roles.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;roles,verbs=get;list;watch;create;update;patch;delete;escalate;bind
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings;rolebindings,verbs=get;list;watch;create;update;patch;delete
// CRDs: operator manages the dra-driver CRDs — no need for wildcard over all apiextensions resources.
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// Apps: cluster-scoped for dra-driver daemonset
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// DeviceClass: operator creates the dra-driver DeviceClass.
// +kubebuilder:rbac:groups=resource.k8s.io,resources=deviceclasses,verbs=get;list;watch;create;update;patch;delete
// SCC: operator creates the SCC that allows the dra-driver privileged pods on OpenShift.
// +kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
// Our own
// +kubebuilder:rbac:groups=ovsdpdk.k8snetworkplumbingwg.io,resources=ovsdpdkoperatorconfigs,verbs=get;list;watch;create;update;patch;delete

func (r *OvsDpdkOperatorConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.log.WithValues("ovsdpdkoperatorconfig", req.NamespacedName)
	r.log.Info("starting reconciliation...")
	operatorConfig := &ovsdpdkoperatorv1alpha1.OvsDpdkOperatorConfig{}
	if err := r.Get(ctx, req.NamespacedName, operatorConfig); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("OvsDpdkOperatorConfig not found", "name", req.Name)
			return ctrl.Result{}, err
		}
		r.log.Error(err, "Failed to get OvsDpdkOperatorConfig", "name", req.Name)
		return ctrl.Result{}, err
	}

	r.log.Info("manifests...")
	if err := r.applyManifests(ctx); err != nil {
		r.log.Error(err, "could not apply dra-driver manifests")
		return ctrl.Result{}, err
	}
	r.log.Info("manifests applied :)")

	// TODO: status checks, updates, etc.

	return ctrl.Result{}, nil
}

func (r *OvsDpdkOperatorConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.log.Info("setting up operator config controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovsdpdkoperatorv1alpha1.OvsDpdkOperatorConfig{}).
		Owns(&appsv1.DaemonSet{}).
		Complete(r)
}

func (r *OvsDpdkOperatorConfigReconciler) applyManifests(ctx context.Context) error {
	if err := r.applyDraDriverCRDS(ctx); err != nil {
		return err
	}

	return r.applyDraDriverManifests(ctx)
}

func (r *OvsDpdkOperatorConfigReconciler) applyDraDriverCRDS(ctx context.Context) error {
	for _, raw := range dependents.GetDraDriverCRDs() {
		r.log.Info("applying crd %s", raw)
		if err := r.applyRawManifests(ctx, raw); err != nil {
			return err
		}
	}
	return nil
}

func (r *OvsDpdkOperatorConfigReconciler) applyDraDriverManifests(ctx context.Context) error {
	r.log.Info("applying manifests %s", dependents.GetDraDriverManifests())
	return r.applyRawManifests(ctx, dependents.GetDraDriverManifests())
}

func (r *OvsDpdkOperatorConfigReconciler) applyRawManifests(
	ctx context.Context,
	manifests []byte,
) error {
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(manifests), 4096)

	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("failed to decode manifest: %w", err)
		} else if len(obj.Object) == 0 {
			continue
		}

		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string, 1)
		}
		labels[consts.OperatorLookupLabelKey] = consts.OperatorLookupLabelValue

		if err = r.applyUnstructuredObj(ctx, obj); err != nil {
			return err
		}
		if obj.GetKind() == "DaemonSet" {
			r.daemonSets = append(r.daemonSets, client.ObjectKeyFromObject(obj))
		}
	}

	return nil
}

// server-side apply, with d
func (r *OvsDpdkOperatorConfigReconciler) applyUnstructuredObj(
	ctx context.Context,
	obj *unstructured.Unstructured,
) error {
	if err := r.Apply(
		context.Background(),
		client.ApplyConfigurationFromUnstructured(obj),
		client.ForceOwnership,
		client.FieldOwner(consts.OperatorFieldOwnership),
	); err != nil {
		return fmt.Errorf("server-side apply of %s failed: %s/%s: %w", obj.GetKind(), obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}
