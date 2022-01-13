/*
Copyright 2022.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodSpec defines the desired state of Pod
type PodSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Pod. Edit pod_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// PodStatus defines the observed state of Pod
type PodStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Pod is the Schema for the pods API
type Pod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodSpec   `json:"spec,omitempty"`
	Status PodStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodList contains a list of Pod
type PodList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Pod `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pod{}, &PodList{})
}
