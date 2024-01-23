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
	"reflect"

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

	pirateshipgraytonwardcomv1alpha1 "github.com/graytonio/pirate-ship/api/v1alpha1"
)

const pirateShipFinalizer = "pirateship.graytonward.com.my.domain/finalizer"

const (
	typeAvailablePirateShip = "Available"

	typeDegradedPirateShip = "Degraded"
)

// PirateShipReconciler reconciles a PirateShip object
type PirateShipReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=pirateships,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=pirateships/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=pirateship.graytonward.com.my.domain,resources=pirateships/finalizers,verbs=update

func (r *PirateShipReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	pirateShip := &pirateshipgraytonwardcomv1alpha1.PirateShip{}
	err := r.Get(ctx, req.NamespacedName, pirateShip)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("pirate ship resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get pirate ship")
		return ctrl.Result{}, err
	}

	if pirateShip.Status.Conditions == nil || len(pirateShip.Status.Conditions) == 0 {
		meta.SetStatusCondition(&pirateShip.Status.Conditions, metav1.Condition{Type: typeAvailablePirateShip, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, pirateShip); err != nil {
			log.Error(err, "Failed to update Radarr status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, pirateShip); err != nil {
			log.Error(err, "Failed to re-fetch radarr")
			return ctrl.Result{}, err
		}
	}

	if !controllerutil.ContainsFinalizer(pirateShip, pirateShipFinalizer) {
		log.Info("Adding finalizer for PirateShip")
		if ok := controllerutil.AddFinalizer(pirateShip, pirateShipFinalizer); !ok {
			log.Error(err, "Failed to add finalizer into the custom resource")
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, pirateShip); err != nil {
			log.Error(err, "Failed to update custom resource to add finalizer")
			return ctrl.Result{}, err
		}
	}

	isPirateShipMarkedToBeDeleted := pirateShip.GetDeletionTimestamp() != nil
	if isPirateShipMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(pirateShip, pirateShipFinalizer) {
			log.Info("Performing Finalizer Operations for PirateShip before delete CR")
			meta.SetStatusCondition(&pirateShip.Status.Conditions, metav1.Condition{
				Type:   typeDegradedPirateShip,
				Status: metav1.ConditionUnknown, Reason: "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", pirateShip.Name)})

			if err := r.Status().Update(ctx, pirateShip); err != nil {
				log.Error(err, "Failed to update PirateShip status")
				return ctrl.Result{}, err
			}

			// DO NOT UNCOMMENT Causes seg faults
			// r.doFinalizerOperationsForPirateShip(pirateShip)

			if err := r.Get(ctx, req.NamespacedName, pirateShip); err != nil {
				log.Error(err, "Failed to re-fetch radarr")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&pirateShip.Status.Conditions, metav1.Condition{Type: typeDegradedPirateShip,
				Status: metav1.ConditionTrue, Reason: "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", pirateShip.Name)})

			if err := r.Status().Update(ctx, pirateShip); err != nil {
				log.Error(err, "Failed to update Radarr status")
				return ctrl.Result{}, err
			}

			log.Info("Removing Finalizer for Radarr after successfully perform the operations")
			if ok := controllerutil.RemoveFinalizer(pirateShip, pirateShipFinalizer); !ok {
				log.Error(err, "Failed to remove finalizer for Radarr")
				return ctrl.Result{Requeue: true}, nil
			}

			if err := r.Update(ctx, pirateShip); err != nil {
				log.Error(err, "Failed to remove finalizer for Radarr")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// TODO(graytonw) Maybe Resolve func for DownloadClient

	err = r.reconcileServarrApplications(ctx, pirateShip)
	if err != nil {
		if err := r.Get(ctx, req.NamespacedName, pirateShip); err != nil {
			log.Error(err, "Failed to re-fetch PirateShip")
			return ctrl.Result{}, err
		}

		meta.SetStatusCondition(&pirateShip.Status.Conditions, metav1.Condition{Type: typeAvailablePirateShip,
			Status: metav1.ConditionFalse, Reason: "Reconciling",
			Message: fmt.Sprintf("Failed to create ServarrApplication for the custom resource (%s): (%s)", pirateShip.Name, err)})

		if err := r.Status().Update(ctx, pirateShip); err != nil {
			log.Error(err, "Failed to update PirateShip status")
			return ctrl.Result{}, err
		}
	}

	meta.SetStatusCondition(&pirateShip.Status.Conditions, metav1.Condition{Type: typeAvailablePirateShip,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: "All Applications deployed successfully",
	})

	if err := r.Status().Update(ctx, pirateShip); err != nil {
		log.Error(err, "Failed to update PirateShip status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PirateShipReconciler) reconcileServarrApplications(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	err := r.reconcileRadarr(ctx, platform)
	if err != nil {
		return err
	}

	err = r.reconcileSonarr(ctx, platform)
	if err != nil {
		return err
	}

	err = r.reconcileReadarr(ctx, platform)
	if err != nil {
		return err
	}

	err = r.reconcileLidarr(ctx, platform)
	if err != nil {
		return err
	}

	err = r.reconcileProwlarr(ctx, platform)
	if err != nil {
		return err
	}

	return nil
}

func (r *PirateShipReconciler) reconcileRadarr(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	log := log.FromContext(ctx)

	found := &pirateshipgraytonwardcomv1alpha1.Radarr{}
	err := r.Get(ctx, types.NamespacedName{Name: r.nameForApplication(platform, "radarr"), Namespace: platform.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if !platform.Spec.Radarr.Enable {
			log.Info("Radarr disabled deleting")
			err = r.Delete(ctx, found)
			if err != nil {
				log.Error(err, "Failed to delete Radarr", "Radarr.Namespace", found.Namespace, "Radarr.Name", found.Name)
				return err
			}
			return nil
		}

		radarr, err := r.radarrForPlatform(platform)
		if err != nil {
			log.Error(err, "Failed to define new Radarr resource for PirateShip")
			return err
		}

		if err = r.Create(ctx, radarr); err != nil {
			log.Error(err, "Failed to create new Radarr",
				"Radarr.Namespace", radarr.Namespace, "Radarr.Name", radarr.Name)
			return err
		}

		return nil
	} else if err != nil {
		log.Error(err, "Failed to get Radarr")
		return err
	}

	if !reflect.DeepEqual(found.Spec, platform.Spec.Radarr.Spec) {
		found.Spec = platform.Spec.Radarr.Spec
		if err = r.Update(ctx, found); err != nil {
			return err
		}
	}

	return nil
}

func (r *PirateShipReconciler) reconcileSonarr(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	log := log.FromContext(ctx)

	found := &pirateshipgraytonwardcomv1alpha1.Sonarr{}
	err := r.Get(ctx, types.NamespacedName{Name: r.nameForApplication(platform, "sonarr"), Namespace: platform.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if !platform.Spec.Sonarr.Enable {
			return nil
		}

		sonarr, err := r.sonarrForPlatform(platform)
		if err != nil {
			log.Error(err, "Failed to define new Sonarr resource for PirateShip")
			return err
		}

		if err = r.Create(ctx, sonarr); err != nil {
			log.Error(err, "Failed to create new Sonarr",
				"Sonarr.Namespace", sonarr.Namespace, "Sonarr.Name", sonarr.Name)
			return err
		}

		return nil
	} else if err == nil && !platform.Spec.Sonarr.Enable {
		log.Info("Sonarr disabled deleting")
		err = r.Delete(ctx, found)
		if err != nil {
			log.Error(err, "Failed to delete Sonarr", "Sonarr.Namespace", found.Namespace, "Sonarr.Name", found.Name)
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to get Sonarr")
		return err
	}

	if !reflect.DeepEqual(found.Spec, platform.Spec.Sonarr.Spec) {
		found.Spec = platform.Spec.Sonarr.Spec
		if err = r.Update(ctx, found); err != nil {
			return err
		}
	}

	return nil
}

func (r *PirateShipReconciler) reconcileReadarr(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	log := log.FromContext(ctx)

	found := &pirateshipgraytonwardcomv1alpha1.Readarr{}
	err := r.Get(ctx, types.NamespacedName{Name: r.nameForApplication(platform, "readarr"), Namespace: platform.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if !platform.Spec.Readarr.Enable {
			log.Info("Readarr disabled deleting")
			err = r.Delete(ctx, found)
			if err != nil {
				log.Error(err, "Failed to delete Readarr", "Readarr.Namespace", found.Namespace, "Readarr.Name", found.Name)
				return err
			}
			return nil
		}

		readarr, err := r.readarrForPlatform(platform)
		if err != nil {
			log.Error(err, "Failed to define new Readarr resource for PirateShip")
			return err
		}

		if err = r.Create(ctx, readarr); err != nil {
			log.Error(err, "Failed to create new Readarr",
				"Readarr.Namespace", readarr.Namespace, "Readarr.Name", readarr.Name)
			return err
		}

		return nil
	} else if err != nil {
		log.Error(err, "Failed to get Readarr")
		return err
	}

	if !reflect.DeepEqual(found.Spec, platform.Spec.Readarr.Spec) {
		found.Spec = platform.Spec.Readarr.Spec
		if err = r.Update(ctx, found); err != nil {
			return err
		}
	}

	return nil
}

func (r *PirateShipReconciler) reconcileLidarr(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	log := log.FromContext(ctx)

	found := &pirateshipgraytonwardcomv1alpha1.Lidarr{}
	err := r.Get(ctx, types.NamespacedName{Name: r.nameForApplication(platform, "lidarr"), Namespace: platform.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		if !platform.Spec.Lidarr.Enable {
			log.Info("Lidarr disabled deleting")
			err = r.Delete(ctx, found)
			if err != nil {
				log.Error(err, "Failed to delete Lidarr", "Lidarr.Namespace", found.Namespace, "Lidarr.Name", found.Name)
				return err
			}
			return nil
		}

		lidarr, err := r.lidarrForPlatform(platform)
		if err != nil {
			log.Error(err, "Failed to define new Lidarr resource for PirateShip")
			return err
		}

		if err = r.Create(ctx, lidarr); err != nil {
			log.Error(err, "Failed to create new Lidarr",
				"Lidarr.Namespace", lidarr.Namespace, "Lidarr.Name", lidarr.Name)
			return err
		}

		return nil
	} else if err != nil {
		log.Error(err, "Failed to get Lidarr")
		return err
	}

	if !reflect.DeepEqual(found.Spec, platform.Spec.Lidarr.Spec) {
		found.Spec = platform.Spec.Lidarr.Spec
		if err = r.Update(ctx, found); err != nil {
			return err
		}
	}

	return nil
}

func (r *PirateShipReconciler) reconcileProwlarr(ctx context.Context, platform *pirateshipgraytonwardcomv1alpha1.PirateShip) error {
	log := log.FromContext(ctx)

	found := &pirateshipgraytonwardcomv1alpha1.Prowlarr{}
	err := r.Get(ctx, types.NamespacedName{Name: r.nameForApplication(platform, "prowlarr"), Namespace: platform.Namespace}, found)
	if err != nil && apierrors.IsNotFound(err) {
		prowlarr, err := r.prowlarrForPlatform(platform)
		if err != nil {
			log.Error(err, "Failed to define new Prowlarr resource for PirateShip")
			return err
		}

		if err = r.Create(ctx, prowlarr); err != nil {
			log.Error(err, "Failed to create new Prowlarr",
				"Prowlarr.Namespace", prowlarr.Namespace, "Prowlarr.Name", prowlarr.Name)
			return err
		}

		return nil
	} else if err != nil {
		log.Error(err, "Failed to get Prowlarr")
		return err
	}

	if !reflect.DeepEqual(found.Spec, platform.Spec.Prowlarr.Spec) {
		found.Spec = platform.Spec.Prowlarr.Spec
		if err = r.Update(ctx, found); err != nil {
			return err
		}
	}

	return nil
}

func (r *PirateShipReconciler) nameForApplication(platform *pirateshipgraytonwardcomv1alpha1.PirateShip, application string) string {
	return fmt.Sprintf("%s-%s", platform.Name, application)
}

func (r *PirateShipReconciler) radarrForPlatform(platform *pirateshipgraytonwardcomv1alpha1.PirateShip) (*pirateshipgraytonwardcomv1alpha1.Radarr, error) {
	radarr := &pirateshipgraytonwardcomv1alpha1.Radarr{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.nameForApplication(platform, "radarr"),
			Namespace: platform.Namespace,
		},
		Spec: platform.Spec.Radarr.Spec,
	}

	if err := ctrl.SetControllerReference(platform, radarr, r.Scheme); err != nil {
		return nil, err
	}

	return radarr, nil
}

func (r *PirateShipReconciler) prowlarrForPlatform(platform *pirateshipgraytonwardcomv1alpha1.PirateShip) (*pirateshipgraytonwardcomv1alpha1.Prowlarr, error) {
	prowlarr := &pirateshipgraytonwardcomv1alpha1.Prowlarr{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.nameForApplication(platform, "prowlarr"),
			Namespace: platform.Namespace,
		},
		Spec: platform.Spec.Prowlarr.Spec,
	}

	if err := ctrl.SetControllerReference(platform, prowlarr, r.Scheme); err != nil {
		return nil, err
	}

	return prowlarr, nil
}

func (r *PirateShipReconciler) sonarrForPlatform(platform *pirateshipgraytonwardcomv1alpha1.PirateShip) (*pirateshipgraytonwardcomv1alpha1.Sonarr, error) {
	sonarr := &pirateshipgraytonwardcomv1alpha1.Sonarr{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.nameForApplication(platform, "sonarr"),
			Namespace: platform.Namespace,
		},
		Spec: platform.Spec.Sonarr.Spec,
	}

	if err := ctrl.SetControllerReference(platform, sonarr, r.Scheme); err != nil {
		return nil, err
	}

	return sonarr, nil
}

func (r *PirateShipReconciler) readarrForPlatform(platform *pirateshipgraytonwardcomv1alpha1.PirateShip) (*pirateshipgraytonwardcomv1alpha1.Readarr, error) {
	readarr := &pirateshipgraytonwardcomv1alpha1.Readarr{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.nameForApplication(platform, "readarr"),
			Namespace: platform.Namespace,
		},
		Spec: platform.Spec.Readarr.Spec,
	}

	if err := ctrl.SetControllerReference(platform, readarr, r.Scheme); err != nil {
		return nil, err
	}

	return readarr, nil
}

func (r *PirateShipReconciler) lidarrForPlatform(platform *pirateshipgraytonwardcomv1alpha1.PirateShip) (*pirateshipgraytonwardcomv1alpha1.Lidarr, error) {
	lidarr := &pirateshipgraytonwardcomv1alpha1.Lidarr{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.nameForApplication(platform, "lidarr"),
			Namespace: platform.Namespace,
		},
		Spec: platform.Spec.Lidarr.Spec,
	}

	if err := ctrl.SetControllerReference(platform, lidarr, r.Scheme); err != nil {
		return nil, err
	}

	return lidarr, nil
}

func (r *PirateShipReconciler) doFinalizerOperationsForPirateShip(cr *pirateshipgraytonwardcomv1alpha1.PirateShip) {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	r.Recorder.Event(cr, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			cr.Name,
			cr.Namespace))
}

// SetupWithManager sets up the controller with the Manager.
func (r *PirateShipReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pirateshipgraytonwardcomv1alpha1.PirateShip{}).
		Complete(r)
}
