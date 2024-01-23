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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Readarr is the Schema for the readarrs API
type Readarr struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServarrSpec   `json:"spec,omitempty"`
	Status ServarrStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ReadarrList contains a list of Readarr
type ReadarrList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Readarr `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Readarr{}, &ReadarrList{})
}

func (r *Readarr) GetAppName() string {
	return "radarr"
}

func (r *Readarr) GetDBNames() []string {
	return []string{
		"readarr-main",
		"readarr-log",
		"readarr-cache",
	}
}

func (r *Readarr) GetStatus() *ServarrStatus {
	return &r.Status
}

func (r *Readarr) GetSpec() *ServarrSpec {
	return &r.Spec
}
