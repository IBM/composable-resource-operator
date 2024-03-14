/**
 * (C) Copyright IBM Corp. 2024.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controller

import (
	"context"
	"errors"
	"github.com/IBM/composable-resource-operator/api/v1alpha1"
	"github.com/IBM/composable-resource-operator/internal/cdi"
	"github.com/IBM/composable-resource-operator/internal/cdi/sunfish"
	util "github.com/IBM/composable-resource-operator/internal/utils"
	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

type ComposabilityRequestAdapter struct {
	instance    *v1alpha1.ComposabilityRequest
	logger      logr.Logger
	client      client.Client
	clientSet   *kubernetes.Clientset
	cdiProvider cdi.CdiProvider
}

const (
	requeueTimeError = 5 * time.Second
	requeueTimePoll  = 1 * time.Second
)

// NewComposabilityRequestAdapter return a new adapter
func NewComposabilityRequestAdapter(instance *v1alpha1.ComposabilityRequest, logger logr.Logger, client client.Client, clientSet *kubernetes.Clientset) *ComposabilityRequestAdapter {
	var cdiProvider cdi.CdiProvider
	switch cdiProviderType := os.Getenv("CDI_PROVIDER_TYPE"); cdiProviderType {
	case "SUNFISH":
		cdiProvider = sunfish.NewSunfishClient()
	default:
		log.Fatal(errors.New("CDI_PROVIDER_TYPE variable not set properly"))
	}

	return &ComposabilityRequestAdapter{instance, logger, client, clientSet, cdiProvider}
}

// EnsureStateSetup is checking that the connection state of the CRD is properly reflected to the
// cluster
func (adapter *ComposabilityRequestAdapter) EnsureStateSetup() (util.OperationResult, error) {
	if adapter.instance.Status.State == v1alpha1.StateEmpty {

		setupLog.Info("reserving the targetNode", "Name", adapter.instance.Name)
		needsUpdate, err := util.ReserveNodesOfTheCRD(adapter.clientSet, adapter.instance)
		if err != nil {
			if errors.Is(err, util.NodeAlreadyReservedError) {
				setupLog.Info("nodes of the crd are already reserved",
					"Name", adapter.instance.Name,
					"targetNode", adapter.instance.Spec.TargetNode,
					"resourceVersionTargetNode", adapter.instance.Annotations["resourceVersionTargetNode"])
				return util.RequeueAfter(requeueTimePoll, nil)
			}
			return util.RequeueWithError(err)
		}
		if needsUpdate {
			setupLog.Info("updating CRD with targetNode resource version",
				"Name", adapter.instance.Name,
				"targetNode", adapter.instance.Spec.TargetNode,
				"resourceVersionTargetNode", adapter.instance.Annotations["resourceVersionTargetNode"])
			return adapter.RequeueUpdateCRD()
		} else {
			setupLog.Info("resourceVersions already in CRD", "Name", adapter.instance.Name)
		}

		hasPendingPods, err := util.NodeHasPendingPods(adapter.clientSet, adapter.instance.Spec.TargetNode)
		if err != nil {
			return util.RequeueWithError(err)
		}
		if hasPendingPods {
			return util.RequeueAfter(requeueTimePoll, nil)
		}
		setupLog.Info("request resource creation", "Name", adapter.instance.Name)
		//attach resource
		if err = adapter.cdiProvider.AddResource(adapter.instance); err != nil {
			adapter.logger.Error(err, "error while sending a sunfish composition request")
			return adapter.RequeueStatusUpdate(v1alpha1.StateFailed)
		}

		setupLog.Info("request resource creation", "Name", adapter.instance.Name)

		setupLog.Info("successfully sent creation request", "Name", adapter.instance.Name)

		return adapter.RequeueStatusUpdate(v1alpha1.StatePending)

	} else if adapter.instance.Status.State == v1alpha1.StatePending {

		//waitForDrivers, err := util.SetDriverTags(adapter.clientSet, adapter.instance, "true")
		//if err != nil {
		//	//error getting the node or updating the node
		//	return util.RequeueWithError(err)
		//}
		//if waitForDrivers {
		//	return adapter.RequeueStatusUpdate(v1alpha1.StateFinalizingSetup)
		//}
		//} else if adapter.instance.Status.State == v1alpha1.StateFinalizingSetup {
		visible, err := util.IsGpusVisible(adapter.clientSet, adapter.instance)
		if visible {
			return adapter.RequeueStatusUpdate(v1alpha1.StateOnline)
		}
		if err != nil {
			//error getting the node or updating the node
			return util.RequeueWithError(err)
		}
		if !visible {
			return util.RequeueAfter(requeueTimePoll, nil)
		}

	} else if adapter.instance.Status.State == v1alpha1.StateFailed {
		setupLog.Info("unreserving the targetNode", "Name", adapter.instance.Name)
		_, err := util.UnReserveNodesOfTheCRD(adapter.clientSet, adapter.instance, false)
		if err != nil {
			return util.RequeueWithError(err)
		}
	}

	return util.ContinueProcessing()

}

// RequeueUpdateCRD Requeues the Reconcile loop appropriately while updating the composability request
func (adapter *ComposabilityRequestAdapter) RequeueUpdateCRD() (result util.OperationResult, err error) {
	updateOptsCRD := &client.UpdateOptions{}
	setupLog.Info("request CRD update", "Name", adapter.instance.Name, "Annotations", adapter.instance.Annotations)
	err = adapter.client.Update(context.TODO(), adapter.instance, updateOptsCRD)
	if err != nil {
		return util.RequeueAfter(requeueTimeError, err)
	}
	setupLog.Info("successfully updated", "Name", adapter.instance.Name, "Status", adapter.instance.Status.State)
	return util.Requeue()
}

// RequeueStatusUpdate Requeues the Reconcile loop appropriately while updating the status of a composability request
func (adapter *ComposabilityRequestAdapter) RequeueStatusUpdate(status string) (result util.OperationResult, err error) {
	adapter.instance.Status.State = status
	setupLog.Info("request status update", "Name", adapter.instance.Name, "Status", adapter.instance.Status.State)
	err = adapter.client.Status().Update(context.TODO(), adapter.instance)

	if err != nil {
		return util.RequeueAfter(requeueTimeError, err)
	}
	setupLog.Info("successfully updated", "Name", adapter.instance.Name, "Status", adapter.instance.Status.State)
	return util.Requeue()
}

// EnsureComposabilityRequestDeletionProcessed is checking if the CRD has been deleted. If yes, it removes
// the CRD finalizer and stops the adapter queue. Otherwise it continues to the next function in the adapter
// queue
func (adapter *ComposabilityRequestAdapter) EnsureComposabilityRequestDeletionProcessed() (util.OperationResult, error) {
	if adapter.instance.DeletionTimestamp == nil {
		return util.ContinueProcessing()
	}
	if adapter.instance.Status.State == v1alpha1.StateOnline {
		visible, err := util.IsGpusVisible(adapter.clientSet, adapter.instance)

		if visible {

			needsUnscheduleUpdate, err := util.SetNodeUnschedulable(adapter.clientSet, adapter.instance)
			if err != nil {
				//error getting the node or updating the node
				return util.RequeueWithError(err)
			}
			if needsUnscheduleUpdate {
				return util.RequeueAfter(requeueTimePoll, nil)
			}
			//node is Unschedulable

			//delete drivers
			waitForDaemonsets, err := util.StopDaemonsets(adapter.clientSet, adapter.instance)
			if err != nil {
				//error getting the node or updating the node
				return util.RequeueWithError(err)
			}
			if waitForDaemonsets {
				return util.RequeueAfter(requeueTimePoll, nil)
			}

			waitForNvidia, err := util.NvidiaDaemonsetsPresent(adapter.clientSet, adapter.instance, "false")
			if err != nil {
				//error getting the node or updating the node
				return util.RequeueWithError(err)
			}
			if waitForNvidia {
				setupLog.Info("waiting for nvidia daemonsets to stop")
				return util.RequeueAfter(requeueTimePoll, nil)
			}

			visible, err := util.IsGpusVisible(adapter.clientSet, adapter.instance)
			if !visible {
				return util.RequeueAfter(requeueTimePoll, nil)
			}
			if err != nil {
				//error getting the node or updating the node
				return util.RequeueWithError(err)
			}

			//EXTRA: evict pods using gpus
			hasGpuPods, err := util.NodeHasGpuPods(adapter.clientSet, adapter.instance.Spec.TargetNode)
			if err != nil {
				return util.RequeueWithError(err)
			}
			if hasGpuPods {
				setupLog.Info("node has pending gpu pods, requeue...")
				return util.RequeueAfter(requeueTimePoll, nil)
			}

		}
		if err != nil {
			return util.RequeueWithError(err)
		}

		if err = adapter.cdiProvider.RemoveResource(adapter.instance); err != nil {
			adapter.logger.Error(err, "error while sending a sunfish composition request")
			return adapter.RequeueStatusUpdate(v1alpha1.StateFailed)
		}

		setupLog.Info("request resource deleted", "Name", adapter.instance.Name)
		return adapter.RequeueStatusUpdate(v1alpha1.StateCleanup)

	} else if adapter.instance.Status.State == v1alpha1.StateCleanup {

		waitForLabels, err := util.RestoreLabels(adapter.clientSet, adapter.instance)
		if err != nil {
			//error getting the node or updating the node
			return util.RequeueWithError(err)
		}
		if waitForLabels {
			return util.RequeueAfter(requeueTimePoll, nil)
		}

		//set as schedulable again
		needsScheduleUpdate, err := util.SetNodeSchedulable(adapter.clientSet, adapter.instance)
		if err != nil {
			//error getting the node or updating the node
			return util.RequeueWithError(err)
		}
		if needsScheduleUpdate {
			return util.RequeueAfter(requeueTimePoll, nil)
		}
		if controllerutil.ContainsFinalizer(adapter.instance, composabilityFinalizer) {
			controllerutil.RemoveFinalizer(adapter.instance, composabilityFinalizer)
			err := adapter.client.Update(context.TODO(), adapter.instance)
			if err != nil {
				return util.RequeueAfter(requeueTimeError, err)
			}
		}
		// here StopProcessing because the instance is
		// deleted
		return util.StopProcessing()

	}

	return util.ContinueProcessing()
}

// EnsureFinalizer is checking if the CRD has the appropriate finalizer. If yes, it continues to the next
// function in the adapter queue. Otherwise it adds the finalizer and does a requeue
func (adapter *ComposabilityRequestAdapter) EnsureFinalizer() (util.OperationResult, error) {
	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(adapter.instance, composabilityFinalizer) {
		controllerutil.AddFinalizer(adapter.instance, composabilityFinalizer)
		err := adapter.client.Update(context.TODO(), adapter.instance)
		if err != nil {
			return util.RequeueAfter(requeueTimeError, err)
		}
		return util.Requeue()
	}
	return util.ContinueProcessing()
}
