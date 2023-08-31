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

package controllers

import (
	"context"
	"fmt"

	applicationv1alpha1 "github.com/graytonio/servarr-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RadarrReconciler reconciles a Radarr object
type RadarrReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

const radarrImageRepo = "ghcr.io/linuxserver/radarr"

const radarrFilalizer = "application.graytonward.com/finalizer"

const (
	typeAvailableRadarr = "Available"
	typeDegradedRadarr  = "Degraded"
)

//+kubebuilder:rbac:groups=application.graytonward.com,resources=radarrs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=application.graytonward.com,resources=radarrs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=application.graytonward.com,resources=radarrs/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Radarr object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *RadarrReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// TODO(user): your logic here
	radarr := &applicationv1alpha1.Radarr{}
	err := r.Get(ctx, req.NamespacedName, radarr)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("radarr resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get radarr")
		return ctrl.Result{}, err
	}

	// Set status as unknown when there is no status available
	if radarr.Status.Conditions == nil || len(radarr.Status.Conditions) == 0 {
		meta.SetStatusCondition(&radarr.Status.Conditions, metav1.Condition{Type: typeAvailableRadarr, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting Reconciliation"})
		if err = r.Status().Update(ctx, radarr); err != nil {
			log.Error(err, "Failed to update Radarr status")
			return ctrl.Result{}, err
		}

		// Let's re-fetch the radarr Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, radarr); err != nil {
			log.Error(err, "Failed to re-fetch radarr")
			return ctrl.Result{}, err
		}
	}

	// Setup a finalizer
	if !controllerutil.ContainsFinalizer(radarr, radarrFilalizer) {
		log.Info("Adding finalizer for radarr")
		if ok := controllerutil.AddFinalizer(radarr, radarrFilalizer); !ok {
			log.Error(err, "Failed to add finalizer into customer resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, radarr); err != nil {
			log.Error(err, "Failed to update cusom resource to add finalizer")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Added finalizer to radarr")
	}

	isRadarrMarkedToBeDeleted := radarr.GetDeletionTimestamp() != nil
	if isRadarrMarkedToBeDeleted {
		return r.deleteRadarrInstanceOperations(ctx, radarr, req)
	}

	res, err := r.reconcileDeployment(ctx, radarr, req)
	if err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

func (r *RadarrReconciler) reconcileDeployment(ctx context.Context, cr *applicationv1alpha1.Radarr, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check for existing deployment
	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, found)

	if err != nil && apierrors.IsNotFound(err) {
		//Create new deployment
		dep, err := r.createRadarrDeployment(cr)
		if err != nil {
			log.Error(err, "Failed to define new Deployment resource for Radarr")

			// The following implementation will update the status
			meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{Type: typeAvailableRadarr,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", cr.Name, err)})

			if err := r.Status().Update(ctx, cr); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			log.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	} else if err != nil {
		log.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	}

	// Update with latest volume specs
	found.Spec.Template.Spec.Volumes = r.getConfiguredVolumes(cr)

	if err = r.Update(ctx, found); err != nil {
		log.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

		// Re-fetch the memcached Custom Resource before update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
			log.Error(err, "Failed to re-fetch memcached")
			return ctrl.Result{}, err
		}

		// The following implementation will update the status
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{Type: typeAvailableRadarr,
			Status: metav1.ConditionFalse, Reason: "Resizing",
			Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", cr.Name, err)})

		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update Memcached status")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, err
	}

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{Type: typeAvailableRadarr,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) created successfully", cr.Name)})

	if err := r.Status().Update(ctx, cr); err != nil {
		log.Error(err, "Failed to update Radarr status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func (r *RadarrReconciler) createRadarrDeployment(radarr *applicationv1alpha1.Radarr) (*appsv1.Deployment, error) {
	var replicas int32 = 1
	deploymentLabels := r.getRadarrDeploymentLabels(radarr)

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      radarr.Name,
			Namespace: radarr.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: deploymentLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: deploymentLabels,
				},
				Spec: corev1.PodSpec{
					Volumes: r.getConfiguredVolumes(radarr),
					Containers: []corev1.Container{{
						Image:           r.getRadarrImage(radarr),
						Name:            "radarr",
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{{
							ContainerPort: radarr.Spec.Port,
							Name:          "http",
						}},
					}},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(radarr, dep, r.Scheme); err != nil {
		return nil, err
	}

	return dep, nil
}

func (r *RadarrReconciler) getConfiguredVolumes(radarr *applicationv1alpha1.Radarr) (vols []corev1.Volume) {
	if radarr.Spec.Config != nil {
		vols = append(vols, *radarr.Spec.Config)
	}

	if radarr.Spec.Media != nil {
		vols = append(vols, *radarr.Spec.Media)
	}

	return vols
}

func (r *RadarrReconciler) getRadarrDeploymentLabels(radarr *applicationv1alpha1.Radarr) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "Radarr",
		"app.kubernetes.io/instance":   radarr.Name,
		"app.kubernetes.io/version":    radarr.Spec.Version,
		"app.kubernetes.io/part-of":    "pirate-ship-operator",
		"app.kubernetes.io/created-by": "controller-manager",
	}
}

func (r *RadarrReconciler) getRadarrImage(radarr *applicationv1alpha1.Radarr) string {
	return fmt.Sprintf("%s:%s", radarrImageRepo, radarr.Spec.Version)
}

func (r *RadarrReconciler) deleteRadarrInstanceOperations(ctx context.Context, cr *applicationv1alpha1.Radarr, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(cr, radarrFilalizer) {
		log.Info("Performing finalizer operations for Radarr before delete CR")

		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{Type: typeDegradedRadarr, Status: metav1.ConditionUnknown, Reason: "Finalizing", Message: fmt.Sprintf("Performing finalizer operations for the custome resource: %s", cr.Name)})
		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update Radarr status")
			return ctrl.Result{}, err
		}

		r.doFinalizerOperationsForRadarr(cr)

		if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
			log.Error(err, "Failed to refetch radarr")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{Type: typeDegradedRadarr,
			Status: metav1.ConditionTrue, Reason: "Finalizing",
			Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", cr.Name)})

		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update Memcached status")
			return ctrl.Result{}, err
		}

		log.Info("Removing Finalizer for Memcached after successfully perform the operations")
		if ok := controllerutil.RemoveFinalizer(cr, radarrFilalizer); !ok {
			log.Error(nil, "Failed to remove finalizer for Memcached")
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, cr); err != nil {
			log.Error(err, "Failed to remove finalizer for Memcached")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *RadarrReconciler) doFinalizerOperationsForRadarr(cr *applicationv1alpha1.Radarr) {
	r.Recorder.Event(cr, "Warnging", "Deleting", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s", cr.Name, cr.Namespace))
}

// SetupWithManager sets up the controller with the Manager.
func (r *RadarrReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&applicationv1alpha1.Radarr{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
