/*
Copyright 2023.

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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RadarrSpec defines the desired state of Radarr
type RadarrSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Application Version to deploy
	//+kubebuilder:default=latest
	//+optional
	Version string `json:"version,omitempty"`

	// Service port to use
	//+kubebuilder:default=7878
	Port int32 `json:"port,omitempty"`

	// Volume to store configuration data
	//+kubebuilder:validation:Optional
	//+kubebuilder:default={}
	Config *v1.Volume `json:"config,omitempty"`

	// Volume that media files are stored
	//+kubebuilder:validation:Optional
	//+kubebuilder:default={}
	Media *v1.Volume `json:"media,omitempty"`
}

// RadarrStatus defines the observed state of Radarr
type RadarrStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//+operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Radarr is the Schema for the radarrs API
type Radarr struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RadarrSpec   `json:"spec,omitempty"`
	Status RadarrStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RadarrList contains a list of Radarr
type RadarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Radarr `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Radarr{}, &RadarrList{})
}
