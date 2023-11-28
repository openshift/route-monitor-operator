/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BlackBoxExporterSpec defines the desired state of BlackBoxExporter
type BlackBoxExporterSpec struct {
	Image string `json:"image,omitempty"`

	// NodeSelector defines which nodes is the BlackBoxExporter gonna run on
	NodeSelector corev1.NodeSelector `json:"nodeSelector,omitempty"`
}

// BlackBoxExporterStatus defines the observed state of BlackBoxExporter
type BlackBoxExporterStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BlackBoxExporter is the Schema for the blackboxexporters API
type BlackBoxExporter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlackBoxExporterSpec   `json:"spec,omitempty"`
	Status BlackBoxExporterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BlackBoxExporterList contains a list of BlackBoxExporter
type BlackBoxExporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlackBoxExporter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BlackBoxExporter{}, &BlackBoxExporterList{})
}
