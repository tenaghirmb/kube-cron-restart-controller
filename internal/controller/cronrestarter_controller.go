/*
Copyright 2026.

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

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	log "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cronrestartv1 "uni.com/cronrestart/api/v1"
	"uni.com/cronrestart/pkg/constants"
)

// CronRestarterReconciler reconciles a CronRestarter object
type CronRestarterReconciler struct {
	client.Client
	APIReader     client.Reader
	EventRecorder record.EventRecorder
	scheme        *runtime.Scheme
	CronManager   *CronManager
}

func NewCronRestarterReconciler(mgr manager.Manager) *CronRestarterReconciler {
	cm := NewCronManager(mgr.GetConfig(), mgr.GetClient())
	r := &CronRestarterReconciler{
		Client:        mgr.GetClient(),
		APIReader:     mgr.GetAPIReader(),
		EventRecorder: mgr.GetEventRecorderFor("CronRestarter"),
		scheme:        mgr.GetScheme(),
		CronManager:   cm,
	}

	err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		cm.Run(ctx)
		return nil
	}))

	if err != nil {
		panic(fmt.Sprintf("unable to add CronManager to manager: %v", err))
	}

	return r
}

var _ reconcile.Reconciler = &CronRestarterReconciler{}

// +kubebuilder:rbac:groups=autorestart.uni.com,resources=cronrestarters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autorestart.uni.com,resources=cronrestarters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autorestart.uni.com,resources=cronrestarters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CronRestarter object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *CronRestarterReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// Refactored from Aggregate Model to SRRM for distributed safety.
	// 1. Fetch the CronRestarter instance
	log.Infof("Start to handle cronRestarter %s in %s namespace", request.Name, request.Namespace)

	instance := &cronrestartv1.CronRestarter{}
	if err := r.Get(ctx, request.NamespacedName, instance); err != nil {
		// Error reading the object - requeue the request.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cronId := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(request.Namespace+request.Name)).String()

	// 2. Check if the cronRestarter is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	if instance.GetDeletionTimestamp() != nil {
		log.Infof("cronRestarter %s in %s namespace is marked to be deleted", request.Name, request.Namespace)
		if controllerutil.ContainsFinalizer(instance, constants.FinalizerName) {
			// Run finalization logic for cronRestarter. If the finalization logic fails,
			// don't remove the finalizer so that we can retry during the next reconciliation.
			if err := r.CronManager.delete(cronId); err != nil {
				log.Errorf("Failed to finalize cronRestarter %s in %s namespace, because of %v", request.Name, request.Namespace, err)
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(instance, constants.FinalizerName)
			if err := r.Update(ctx, instance); err != nil {
				log.Errorf("Failed to remove finalizer for cronRestarter %s in %s namespace, because of %v", request.Name, request.Namespace, err)
				return ctrl.Result{}, err
			}
			log.Infof("Remove finalizer for cronRestarter %s in %s namespace", request.Name, request.Namespace)
		}
		log.Infof("cronRestarter %s in %s namespace has been finalized successfully. cronId: %s", request.Name, request.Namespace, cronId)
		return ctrl.Result{}, nil
	}

	// 3. Create or update the cron job in cron manager according to the spec of cronRestarter
	if !controllerutil.ContainsFinalizer(instance, constants.FinalizerName) {
		controllerutil.AddFinalizer(instance, constants.FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			log.Errorf("Failed to add finalizer for cronRestarter %s in %s namespace, because of %v", request.Name, request.Namespace, err)
			return ctrl.Result{}, err
		}
		log.Infof("Add finalizer for cronRestarter %s in %s namespace", request.Name, request.Namespace)
	}

	job, err := CronRestarterJobFactory(instance, r.Client, r.APIReader, r.EventRecorder)
	if err != nil {
		log.Errorf("Failed to create cron job for cronRestarter %s in %s namespace, because %v", request.Name, request.Namespace, err)
		return ctrl.Result{}, err
	}
	job.SetID(cronId)
	err = r.CronManager.createOrUpdate(job)
	if err != nil {
		log.Errorf("Failed to create or update cron job for cronRestarter %s in %s namespace, because %v", request.Name, request.Namespace, err)
		return ctrl.Result{}, err
	}

	instance.Status.State = cronrestartv1.Submitted
	if err := r.Status().Update(ctx, instance); err != nil {
		log.Errorf("Failed to mark cronRestarter %s in %s namespace as submitted, because of %v", request.Name, request.Namespace, err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronRestarterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cronrestartv1.CronRestarter{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}). // ignore resourceVersion/Status update
		Complete(r)
}
