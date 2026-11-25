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
	"context"
	"fmt"

	"k8s.io/klog/v2"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	nmstatev1 "github.com/nmstate/kubernetes-nmstate/api/v1"

	ovsdpdkoperatorv1alpha1 "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/api/ovsdpdkoperator/v1alpha1"

	nmstateutil "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/nmstate"
)

type OvsDpdkBridgeBindingController struct {
	client.Client
	log klog.Logger
}

func NewOvsDpdkBridgeBindingController(
	c client.Client,
) *OvsDpdkBridgeBindingController {
	return &OvsDpdkBridgeBindingController{
		Client: c,
		log:    klog.Background().WithName("OvsDpdkBridgeBindingController"),
	}
}

// nmstate's
// +kubebuilder:rbac:groups=nmstate.io,resources=nodenetworkconfigurationpolicies,verbs=get;list;watch;create;update;patch;delete
// Our own
// +kubebuilder:rbac:groups=ovsdpdk.k8snetworkplumbingwg.io,resources=ovsdpdkbridgebindings,verbs=get;list;watch;create;update;patch;delete

func (r *OvsDpdkBridgeBindingController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.log.WithValues("ovsdpdkbridgebinding", req.NamespacedName)
	r.log.Info("a bb is on its way?")
	bb := &ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding{}
	if err := r.Get(ctx, req.NamespacedName, bb); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("bb not found", "name", req.Name)
			return ctrl.Result{}, err
		}
		r.log.Error(err, "Failed to get bb", "name", req.Name)
		return ctrl.Result{}, err
	}

	if err := r.applyNncp(ctx, bb); err != nil {
		r.log.Error(err, "can't create NNCP", "name", req.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OvsDpdkBridgeBindingController) SetupWithManager(mgr ctrl.Manager) error {
	r.log.Info("setting up operator config controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding{}).
		Owns(&nmstatev1.NodeNetworkConfigurationPolicy{}).
		Complete(r)
}

func (r *OvsDpdkBridgeBindingController) applyNncp(
	ctx context.Context,
	bb *ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding,
) error {
	nncpRender := nmstateutil.RenderNNCPFromBB(bb)
	nncpRender.SetGroupVersionKind(nmstatev1.GroupVersion.WithKind("NodeNetworkConfigurationPolicy"))

	if err := controllerutil.SetControllerReference(bb, nncpRender, r.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference on NNCP %s: %w", nncpRender.GetName(), err)
	}
	r.log.Info("trying to apply NNCP", "name", nncpRender.GetName(), "desiredState", string(nncpRender.Spec.DesiredState.Raw))

	if err := r.Patch(
		ctx,
		nncpRender,
		client.Apply,
		client.FieldOwner("ovsdpdk-bridge-binding-controller"),
		client.ForceOwnership,
	); err != nil {
		return fmt.Errorf("failed to apply NNCP %s: %w", nncpRender.GetName(), err)
	}

	return nil
}
