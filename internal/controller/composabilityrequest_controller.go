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

package controller

import (
	"context"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	crov1alpha1 "github.com/IBM/composable-resource-operator/api/v1alpha1"
	util "github.com/IBM/composable-resource-operator/internal/utils"
)

const composabilityFinalizer = "com.ie.ibm.hpsys/finalizer"

// ComposabilityRequestReconciler reconciles a ComposabilityRequest object
type ComposabilityRequestReconciler struct {
	client.Client
	ClientSet *kubernetes.Clientset
	Log       logr.Logger
	Scheme    *runtime.Scheme
}

var (
	setupLog = ctrl.Log.WithName("composability_controller")
)

//+kubebuilder:rbac:groups=cro.hpsys.ibm.ie.com,resources=composabilityrequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cro.hpsys.ibm.ie.com,resources=composabilityrequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cro.hpsys.ibm.ie.com,resources=composabilityrequests/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;patch;update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;patch;update;list;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ComposabilityRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *ComposabilityRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	setupLog.Info("Triggered reconcile loop...")
	instance := &crov1alpha1.ComposabilityRequest{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			return r.doNotRequeue()
		}
		return r.requeueOnErr(err)
	}
	setupLog.Info("name: ", "string", instance.ObjectMeta.Name)
	setupLog.Info("targetNode: ", "string", instance.Spec)

	setupLog.Info("Current resource ", "version", instance.ObjectMeta.ResourceVersion)
	setupLog.Info("Current resource:", "connection", instance.Status.State)

	adapter := NewComposabilityRequestAdapter(instance, r.Log, r.Client, r.ClientSet)
	result, err := r.ReconcileHandler(adapter)
	return result, err
}

type ReconcileOperation func() (util.OperationResult, error)

func (r *ComposabilityRequestReconciler) ReconcileHandler(adapter *ComposabilityRequestAdapter) (reconcile.Result, error) {
	operations := []ReconcileOperation{
		adapter.EnsureComposabilityRequestDeletionProcessed,
		adapter.EnsureFinalizer,
		adapter.EnsureStateSetup,
	}
	for _, operation := range operations {
		result, err := operation()
		if err != nil || result.RequeueRequest {
			return r.requeueAfter(result.RequeueDelay, err)
		}
		if result.CancelRequest {
			return r.doNotRequeue()
		}
	}
	return r.doNotRequeue()
}

func (r *ComposabilityRequestReconciler) requeueAfter(duration time.Duration, err error) (ctrl.Result, error) {
	return ctrl.Result{RequeueAfter: duration}, err
}

func (r *ComposabilityRequestReconciler) doNotRequeue() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *ComposabilityRequestReconciler) requeueOnErr(err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComposabilityRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crov1alpha1.ComposabilityRequest{}).
		Complete(r)
}
