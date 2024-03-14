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

package utils

import (
	"context"
	"errors"
	"fmt"
	"github.com/IBM/composable-resource-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"strconv"
	"strings"
)

var NodeAlreadyReservedError = errors.New("node is already reserved")

func errNodeAlreadyReservedError() error { return NodeAlreadyReservedError }

func DeletePod(clientSet *kubernetes.Clientset, pod *v1.Pod) error {
	deleteOpts := metav1.DeleteOptions{}

	err := clientSet.CoreV1().Pods(pod.GetObjectMeta().GetNamespace()).Delete(context.TODO(), pod.Name, deleteOpts)

	return err
}

func UpdateNode(clientSet *kubernetes.Clientset, node *v1.Node) error {
	updateOpts := metav1.UpdateOptions{}
	node, err := clientSet.CoreV1().Nodes().Update(context.TODO(), node, updateOpts)

	return err
}

func ReserveNode(clientSet *kubernetes.Clientset, node *v1.Node, instance *v1alpha1.ComposabilityRequest) error {
	return AppendLabelToNode(clientSet, node, "node.kubernetes.io/composability", instance.Name)
}

func AppendLabelToNode(clientSet *kubernetes.Clientset, node *v1.Node, labelKey, labelValue string) error {

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	} else {
		if val, exists := node.Labels[labelKey]; exists {
			if val == labelValue {
				return nil
			} else {
				return errNodeAlreadyReservedError()
			}
		}
	}

	node.Labels[labelKey] = labelValue

	return UpdateNode(clientSet, node)
}

func RemoveLabelFromNode(clientSet *kubernetes.Clientset, node *v1.Node, labelKey, labelValue string) error {

	if node.Labels == nil {
		return nil
	}

	val, ok := node.Labels[labelKey]

	if !ok {
		return nil
	} else {
		if val != labelValue {
			return nil
		}
	}

	delete(node.Labels, labelKey)

	return UpdateNode(clientSet, node)
}

func GetNode(clientSet *kubernetes.Clientset, nodeName string) (*v1.Node, error) {
	getOpts := metav1.GetOptions{}
	node, err := clientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, getOpts)

	if err != nil {
		return nil, err
	}

	return node, nil
}

func ReserveNodesOfTheCRD(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	// First we need to Reserve a node by appending the label (will cause an increment of ResourceVersion),
	// then get the latest ResourceVersion and only afterwards the orchestrator restart the kubelet service
	// (will cause another increment of ResourceVersion)
	crdNeedsUpdate := false
	if request.Spec.TargetNode != "" {
		node, err := GetNode(clientSet, request.Spec.TargetNode)
		if err != nil {
			return false, err
		}
		err = ReserveNode(clientSet, node, request)
		if err != nil {
			return false, err
		}
		if request.Annotations == nil {
			request.Annotations = make(map[string]string)
		}
		val, exists := request.Annotations["resourceVersionTargetNode"]
		if !exists || val != node.ResourceVersion {
			request.Annotations["resourceVersionTargetNode"] = node.ResourceVersion
			crdNeedsUpdate = true
		}
	}

	return crdNeedsUpdate, nil
}

func GetPodsOfNode(clientSet *kubernetes.Clientset, nodeName, namespace string) (*v1.PodList, error) {
	return clientSet.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
}

func NodeHasPendingPods(clientSet *kubernetes.Clientset, nodeName string) (bool, error) {
	pods, err := GetPodsOfNode(clientSet, nodeName, "default")
	if err != nil {
		return false, err
	}

	for _, p := range pods.Items {
		if p.Status.Phase == "Pending" {
			return true, nil
		}
	}

	return false, nil
}

func NodeHasGpuPods(clientSet *kubernetes.Clientset, nodeName string) (bool, error) {
	pods, err := GetPodsOfNode(clientSet, nodeName, "default")
	if err != nil {
		return false, err
	}

	nodeHasGpuPods := false

	for _, p := range pods.Items {
		for _, c := range p.Spec.Containers {
			podHasGpus := false
			_, exists := c.Resources.Requests["nvidia.com/gpu"]
			// If the key exists
			if exists {
				nodeHasGpuPods = true
				podHasGpus = true
				// delete pod
			}
			if podHasGpus {
				err = DeletePod(clientSet, &p)
				if err != nil {
					return true, err
				}
			}
		}
	}

	return nodeHasGpuPods, nil
}

func UnReserveNode(clientSet *kubernetes.Clientset, node *v1.Node, instance *v1alpha1.ComposabilityRequest) error {
	return RemoveLabelFromNode(clientSet, node, "node.kubernetes.io/composability", instance.Name)
}

func UnReserveNodesOfTheCRD(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest, checkOnUpdatedResources bool) (bool, error) {

	// First we need to check if ResourceVersion is increased and then UnReserve the node because the latter will
	// trigger an increment of ResourceVersion on each own

	resourceVersionsUpdated := true

	if request.Spec.TargetNode != "" {
		node, err := GetNode(clientSet, request.Spec.TargetNode)
		if err != nil {
			return false, err
		}
		val, exists := request.Annotations["resourceVersionTargetNode"]
		if checkOnUpdatedResources {
			if request.Annotations != nil && exists {

				annotatedResourceVersion, err := strconv.Atoi(val)
				if err != nil {
					return false, err
				}

				nodeResourceVersion, err := strconv.Atoi(node.ResourceVersion)
				if err != nil {
					return false, err
				}

				if nodeResourceVersion > annotatedResourceVersion {
					err = UnReserveNode(clientSet, node, request)
					fmt.Println("### Sent request to untag the node...")
					if err != nil {
						return false, err
					}
				} else {
					resourceVersionsUpdated = false
				}
			}
		} else {
			err = UnReserveNode(clientSet, node, request)
			if err != nil {
				return false, err
			}
		}
	}

	return resourceVersionsUpdated, nil
}

func SetNodeUnschedulable(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	nodeNeedsUpdates := false
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return false, err
	}
	if node.Spec.Unschedulable == false {
		err := UpdateUnschedulableValue(clientSet, node, true)
		nodeNeedsUpdates = true
		if err != nil {
			return nodeNeedsUpdates, err
		}
		return nodeNeedsUpdates, nil

	}
	return nodeNeedsUpdates, nil
}

func SetNodeSchedulable(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	nodeNeedsUpdates := false
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return false, err
	}
	if node.Spec.Unschedulable == true {
		err := UpdateUnschedulableValue(clientSet, node, false)
		nodeNeedsUpdates = true
		if err != nil {
			return nodeNeedsUpdates, err
		}
		return nodeNeedsUpdates, nil

	}
	return nodeNeedsUpdates, nil
}

func UpdateUnschedulableValue(clientSet *kubernetes.Clientset, node *v1.Node, desiredState bool) error {

	node.Spec.Unschedulable = desiredState

	return UpdateNode(clientSet, node)
}

func isGpusVisible(node *v1.Node) bool {
	if val, exists := node.Status.Allocatable["nvidia.com/gpu"]; exists {
		if val.IsZero() {
			return false
		} else {
			return true
		}
	}
	return false

}

func IsGpusVisible(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return false, err
	}
	return isGpusVisible(node), nil

}
func UninstallDrivers(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	nvidiaLabelKeys := []string{
		"nvidia.com/gpu.deploy.container-toolkit",
		"nvidia.com/gpu.deploy.dcgm",
		"nvidia.com/gpu.deploy.dcgm-exporter",
		"nvidia.com/gpu.deploy.device-plugin",
		"nvidia.com/gpu.deploy.driver",
		//"nvidia.com/gpu.deploy.gpu-feature-discovery",
		//"nvidia.com/gpu.deploy.node-status-exporter",
		//"nvidia.com/gpu.deploy.operator-validator",
	}

	nodeNeedsUpdates := false
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return nodeNeedsUpdates, err
	}
	if !isGpusVisible(node) {
		return nodeNeedsUpdates, nil
	}

	nodeNeedsUpdates = true
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	} else {
		for _, label := range nvidiaLabelKeys {
			if val, exists := node.Labels[label]; exists {
				if val == "false" {
					continue
				}
			}
			node.Labels[label] = "false"
		}
	}
	if nodeNeedsUpdates {
		err := UpdateNode(clientSet, node)
		return nodeNeedsUpdates, err
	}

	return nodeNeedsUpdates, nil
}
func NvidiaDaemonsetsPresent(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest, installStatus string) (bool, error) {
	pods, err := GetPodsOfNode(clientSet, request.Spec.TargetNode, "default")
	if err != nil {
		return false, err
	}

	nvidiaDaemonsets := []string{
		"nvidia-container-toolkit-daemonset",
		"nvidia-device-plugin-daemonset",
		"nvidia-dcgm",
		"nvidia-dcgm-exporter",
	}
	for _, p := range pods.Items {
		for _, o := range p.ObjectMeta.OwnerReferences {
			for _, nd := range nvidiaDaemonsets {
				if o.Name == nd {
					return true, nil
				}
			}
			if strings.Contains(o.Name, "nvidia-driver-daemonset") {
				return true, nil
			}
		}
	}
	return false, nil

}

func StopDaemonsets(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	//TODO: make sure status exported is running
	nvidiaLabelKeys := []string{
		"nvidia.com/gpu.deploy.container-toolkit",
		"nvidia.com/gpu.deploy.device-plugin",
		"nvidia.com/gpu.deploy.driver",
		"nvidia.com/gpu.deploy.gpu-feature-discovery",
		//"nvidia.com/gpu.deploy.operator-validator",
		"nvidia.com/gpu.deploy.dcgm",
		"nvidia.com/gpu.deploy.dcgm-exporter",
		//"nvidia.com/gpu.deploy.node-status-exporter",
	}

	//do not do anything if we are attaching resources, the operator will take care of it

	nodeNeedsUpdates := false
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return nodeNeedsUpdates, err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	} else {
		for _, label := range nvidiaLabelKeys {
			if val, exists := node.Labels[label]; exists {
				if val != "false" {
					node.Labels[label] = "false"
					nodeNeedsUpdates = true
				}
			}
		}
	}
	if nodeNeedsUpdates {
		err := UpdateNode(clientSet, node)
		return nodeNeedsUpdates, err
	}

	return nodeNeedsUpdates, nil
}

func RestoreLabels(clientSet *kubernetes.Clientset, request *v1alpha1.ComposabilityRequest) (bool, error) {

	//TODO: make sure status exported is running
	nvidiaLabelKeys := []string{
		"nvidia.com/gpu.deploy.container-toolkit",
		"nvidia.com/gpu.deploy.device-plugin",
		"nvidia.com/gpu.deploy.driver",
		"nvidia.com/gpu.deploy.gpu-feature-discovery",
		//"nvidia.com/gpu.deploy.operator-validator",
		"nvidia.com/gpu.deploy.dcgm",
		"nvidia.com/gpu.deploy.dcgm-exporter",
		//"nvidia.com/gpu.deploy.node-status-exporter",
	}

	//do not do anything if we are attaching resources, the operator will take care of it

	nodeNeedsUpdates := false
	node, err := GetNode(clientSet, request.Spec.TargetNode)
	if err != nil {
		return nodeNeedsUpdates, err
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	} else {
		for _, label := range nvidiaLabelKeys {
			if _, exists := node.Labels[label]; exists {
				if exists {
					delete(node.Labels, label)
					nodeNeedsUpdates = true
				}
			}
		}
	}
	if nodeNeedsUpdates {
		err := UpdateNode(clientSet, node)
		return nodeNeedsUpdates, err
	}

	return nodeNeedsUpdates, nil
}
