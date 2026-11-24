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

package dependents

import (
	_ "embed"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/yaml"
)

type DaemonSetManifest struct {
	*appsv1.DaemonSet
}

func NewDaemonSetManifestFrom(data []byte) (*DaemonSetManifest, error) {
	raw := &appsv1.DaemonSet{}
	err := yaml.Unmarshal(data, raw)
	if err != nil {
		return nil, err
	}

	return &DaemonSetManifest{DaemonSet: raw}, nil
}

func (ds *DaemonSetManifest) WithNamespace(namespace string) *DaemonSetManifest {
	ds.Namespace = namespace
	return ds
}

// Sets the given container image to the container matching name
func (ds *DaemonSetManifest) WithContainerImage(
	containerName string,
	containerImage string,
) *DaemonSetManifest {
	// FIXME This way of arranging things might not be the best, as this
	// function should actually return an error if the container is not
	// found.
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name != containerName {
			continue
		}

		container.Image = containerImage
	}

	return ds
}

func (ds *DaemonSetManifest) WithInitContainer(
	initContainer *corev1.Container,
) *DaemonSetManifest {
	ds.Spec.Template.Spec.Containers = append(ds.Spec.Template.Spec.Containers, *initContainer)
	return ds
}
