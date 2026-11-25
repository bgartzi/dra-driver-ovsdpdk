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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=ovsdpdkbridgebindings,shortName=bb,scope=Cluster
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.status==\"True\")].type",description="Status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.status==\"True\")].reason",description="Reason"
type OvsDpdkBridgeBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OvsDpdkBridgeBindingSpec   `json:"spec,omitempty"`
	Status OvsDpdkBridgeBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type OvsDpdkBridgeBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OvsDpdkBridgeBinding `json:"items"`
}

type OvsDpdkBridgeBindingSpec struct {
	// NodeSelector is a selector that determines which nodes the policy will be applied to.
	// It uses simple key-value label matching (equality-based selection only). All specified
	// labels must match a node's labels for the policy to be scheduled on that node.
	// Note: matchLabels and matchExpressions are not supported; only direct key-value pair matching is available.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	// +kubebuilder:validation:MaxProperties=256
	//nolint:lll
	// +kubebuilder:validation:XValidation:rule="self.all(k, !format.qualifiedName().validate(k).hasValue())",message="nodeSelector keys must be valid qualified names"
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// BridgeName...
	//
	// +required
	BridgeName string `json:"bridgeName,omitempty"`

	// Uplink Determines how the uplink will look like
	// +required
	Uplink BridgeUplinkSpec `json:"uplink,omitempty"`

	// Mtu determines the mtu :), which will default to 1500
	// +optional
	Mtu uint `json:"mtu,omitempty"`
}

type BridgeUplinkSpec struct {
	// +optional
	Interface UplinkInterfaceSpec `json:"interface,omitempty"`
}

type UplinkInterfaceSpec struct {
	// +optional
	Pci string `json:"pci,omitempty"`
}

type OvsDpdkBridgeBindingStatus struct {
	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the latest available observations of an object's state
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
