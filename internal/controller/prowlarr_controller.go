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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pirateshipgraytonwardcomv1alpha1 "github.com/graytonio/pirate-ship/api/v1alpha1"
)

// ProwlarrReconciler reconciles a Prowlarr object
type ProwlarrReconciler struct {
	client.Client
	ClusterScheme *runtime.Scheme
	Recorder      record.EventRecorder
}

// The following markers are used to generate the rules permissions (RBAC) on config/rbac using controller-gen
// when the command <make manifests> is executed.
// To know more about markers see: https://book.kubebuilder.io/reference/markers.html

//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=prowlarrs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=prowlarrs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=prowlarrs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (r *ProwlarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Radarr instance
	// The purpose is check if the Custom Resource for the Kind Radarr
	// is applied on the cluster if not we return nil to stop the reconciliation
	prowlarr := &pirateshipgraytonwardcomv1alpha1.Prowlarr{}
	res, err := reconcileServarrCRD(r, ctx, req, prowlarr)
	if err != nil || !res.IsZero() {
		return res, err
	}

	// Reconcile DB Config
	res, err = reconcileServarrDB(r, ctx, req, prowlarr)
	if err != nil || !res.IsZero() {
		return res, err
	}

	// Reconcile ConfigMapObject
	res, err = reconcileServarrConfigMap(r, ctx, req, prowlarr)
	if err != nil || !res.IsZero() {
		return res, err
	}

	// Reconcile Deployment Object
	res, err = reconcileServarrDeployment(r, ctx, req, prowlarr)
	if err != nil || !res.IsZero() {
		return res, err
	}

	// The following implementation will update the status
	meta.SetStatusCondition(&prowlarr.Status.Conditions, metav1.Condition{Type: typeAvailableServarr,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with replicas created successfully", prowlarr.Name)})

	if err := r.Status().Update(ctx, prowlarr); err != nil {
		log.Error(err, "Failed to update Prowlarr status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note that the Deployment will be also watched in order to ensure its
// desirable state on the cluster
func (r *ProwlarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pirateshipgraytonwardcomv1alpha1.Prowlarr{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
