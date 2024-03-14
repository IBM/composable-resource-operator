package cdi

import "github.com/IBM/composable-resource-operator/api/v1alpha1"

type CdiProvider interface {
	AddResource(instance *v1alpha1.ComposabilityRequest) error
	RemoveResource(instance *v1alpha1.ComposabilityRequest) error
}
