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
	// +kubebuilder:validation:Enum=infra;hcp
	// +kubebuilder:default:="infra"
	// +optional
	DomainRef ClusterDomainRef `json:"domainRef,omitempty"`

	// +kubebuilder:default:false
	// +kubebuilder:validation:Optional

	// SkipPrometheusRule instructs the controller to skip the creation of PrometheusRule CRs.
	// One common use-case for is for alerts that are defined separately, such as for hosted clusters.
	SkipPrometheusRule bool `json:"skipPrometheusRule"`
}

// ClusterDomainRef defines the object used determine the cluster's domain
// By default, 'infra' is used, which references the 'infrastructures/cluster' object
type ClusterDomainRef string

var (
	// ClusterDomainRefInfra indicates the clusterDomain should be determined from the 'infrastructures/cluster' object
	ClusterDomainRefInfra ClusterDomainRef = "infra"

	// ClusterDomainRefHCP indicates the clusterDomain should be determined from the 'hcp/cluster' object in the same namespace as the ClusterURLMonitor being reconciled
	ClusterDomainRefHCP ClusterDomainRef = "hcp"
)

// ClusterUrlMonitorStatus defines the observed state of ClusterUrlMonitor
type ClusterUrlMonitorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	ServiceMonitorRef NamespacedName `json:"serviceMonitorRef,omitempty"`
	PrometheusRuleRef NamespacedName `json:"prometheusRuleRef,omitempty"`
	ErrorStatus       string         `json:"errorStatus,omitempty"`
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
