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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type ServarrSpec struct {
	// Size defines the number of Servarr instances
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3
	// +kubebuilder:default=1
	// +kubebuilder:validation:ExclusiveMaximum=false
	Size int32 `json:"size,omitempty"`

	// Port defines the port that will be used to init the container with the image
	// +kubebuilder:default=7878
	// +kubebuilder:validation:Optional
	ContainerPort int32 `json:"containerPort,omitempty"`

	// +kubebuilder:default="5.1.3"
	// +kubebuilder:validation:Optional
	Version string `json:"version,omitempty"`

	// +kubebuilder:validation:Required
	DB DBSpec `json:"db,omitempty"`
}

type ServarrStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

type PlatformProwlarr struct {
	Spec ServarrSpec `json:"spec,omitempty"`
}

type PlatformServarr struct {
	Enable bool        `json:"enabled,omitempty"`
	Spec   ServarrSpec `json:"spec,omitempty"`
}

type EnvValue struct {
	Value     string              `json:"value,omitempty"`
	ValueFrom corev1.EnvVarSource `json:"valueFrom,omitempty"`
}

type DBCredentials struct {
	Username EnvValue `json:"username,omitempty"`
	Password EnvValue `json:"password,omitempty"`
}

type DBSpec struct {
	// +kubebuilder:validation:Enum=external
	// +kubebuilder:default=external
	Source string `json:"source,omitempty"`

	Host        string        `json:"host,omitempty"`
	Port        int           `json:"port,omitempty"`
	Credentials DBCredentials `json:"credentials,omitempty"`
}

// PirateShipSpec defines the desired state of PirateShip
type PirateShipSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Radarr   PlatformServarr  `json:"radarr,omitempty"`
	Sonarr   PlatformServarr  `json:"sonarr,omitempty"`
	Lidarr   PlatformServarr  `json:"lidarr,omitempty"`
	Readarr  PlatformServarr  `json:"readarr,omitempty"`
	Prowlarr PlatformProwlarr `json:"prowlarr,omitempty"`
	DB       DBSpec           `json:"db,omitempty"`
}

// PirateShipStatus defines the observed state of PirateShip
type PirateShipStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PirateShip is the Schema for the pirateships API
type PirateShip struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PirateShipSpec   `json:"spec,omitempty"`
	Status PirateShipStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PirateShipList contains a list of PirateShip
type PirateShipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PirateShip `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PirateShip{}, &PirateShipList{})
}
