/*
Copyright 2024.

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

const (
	StateEmpty           = ""
	StateOnline          = "Online"
	StatePending         = "Pending"
	StateFinalizingSetup = "FinalizingSetup"
	StateCleanup         = "Cleanup"
	StateFailed          = "Failed"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ScalarResourceDetails struct {
	Size  int64  `json:"size,omitempty"`
	Model string `json:"model,omitempty"`
}

// FrameworkResources define resources as they are defined inside k8s
type FrameworkResources struct {
	MilliCPU         int64                            `json:"milli_cpu,omitempty"`
	Memory           int64                            `json:"memory,omitempty"`
	EphemeralStorage int64                            `json:"ephemeral_storage,omitempty"`
	AllowedPodNumber int                              `json:"allowed_pod_number,omitempty"`
	ScalarResources  map[string]ScalarResourceDetails `json:"scalar_resources,omitempty"`
}

// ComposabilityRequestSpec defines the desired state of ComposabilityRequest
type ComposabilityRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	TargetNode string             `json:"targetNode"`
	Resources  FrameworkResources `json:"resources"`
}

// ComposabilityRequestStatus defines the observed state of ComposabilityRequest
type ComposabilityRequestStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State string `json:"state"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ComposabilityRequest is the Schema for the composabilityrequests API
type ComposabilityRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComposabilityRequestSpec   `json:"spec,omitempty"`
	Status ComposabilityRequestStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComposabilityRequestList contains a list of ComposabilityRequest
type ComposabilityRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComposabilityRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComposabilityRequest{}, &ComposabilityRequestList{})
}
