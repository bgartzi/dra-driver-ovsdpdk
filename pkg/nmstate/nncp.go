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

package nmstate

import (
	"crypto/sha256"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nmstateshared "github.com/nmstate/kubernetes-nmstate/api/shared"
	nmstatev1 "github.com/nmstate/kubernetes-nmstate/api/v1"

	ovsdpdkoperatorv1alpha1 "github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk/pkg/api/ovsdpdkoperator/v1alpha1"
)

func RenderNNCPFromBB(
	bb *ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding,
) *nmstatev1.NodeNetworkConfigurationPolicy {

	return &nmstatev1.NodeNetworkConfigurationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: bb.Name,
		},
		Spec: nmstateshared.NodeNetworkConfigurationPolicySpec{
			NodeSelector: bb.Spec.NodeSelector,
			DesiredState: nncpDesiredStateFromBB(bb),
		},
	}
}

func nncpDesiredStateFromBB(
	bb *ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding,
) nmstateshared.State {
	return nmstateshared.State{
		Raw: rawStateFromBB(bb),
	}
}

func rawStateFromBB(bb *ovsdpdkoperatorv1alpha1.OvsDpdkBridgeBinding) []byte {
	dpdkPortName := portName(bb.Spec.Uplink.Interface.Pci, bb.Spec.BridgeName)

	return []byte(fmt.Sprintf(
		`ovs-db:
  other_config:
    dpdk-init: "true"
    dpdk-extra: "-a 0000:00:00.0 --log-level debug"
interfaces:
  - name: %s
    type: ovs-interface
    state: up
    dpdk:
      devargs: %s
  - name: %s
    type: ovs-bridge
    state: up
    bridge:
      options:
        datapath: netdev
      port:
      - name: %s
`,
		dpdkPortName,
		bb.Spec.Uplink.Interface.Pci,
		bb.Spec.BridgeName,
		dpdkPortName,
	))
}

func portName(pciAddress, bridgeName string) string {
	return fmt.Sprintf("ovs-dpdk-port-%x%x", hashId(pciAddress), hashId(bridgeName))
}

func hashId(name string) string {
	sum := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", sum[:2])[:3]
}
