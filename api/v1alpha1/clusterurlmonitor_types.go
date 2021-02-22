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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterUrlMonitorSpec defines the desired state of ClusterUrlMonitor
type ClusterUrlMonitorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ClusterUrlMonitor. Edit ClusterUrlMonitor_types.go to remove/update
	Prefix string  `json:"prefix,omitempty"`
	Suffix string  `json:"suffix,omitempty"`
	Port   string  `json:"port,omitempty"`
	Slo    SloSpec `json:"slo,omitempty"`
}

// ClusterUrlMonitorStatus defines the observed state of ClusterUrlMonitor
type ClusterUrlMonitorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ServiceMonitorRef NamespacedName `json:"serviceMonitorRef,omitempty"`
	PrometheusRuleRef NamespacedName `json:"prometheusRuleRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClusterUrlMonitor is the Schema for the clusterurlmonitors API
type ClusterUrlMonitor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterUrlMonitorSpec   `json:"spec,omitempty"`
	Status ClusterUrlMonitorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterUrlMonitorList contains a list of ClusterUrlMonitor
type ClusterUrlMonitorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterUrlMonitor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterUrlMonitor{}, &ClusterUrlMonitorList{})
}
