/*
Copyright 2022 The Kubernetes Authors.

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

package clustermodule

import (
	goctx "context"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/context"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/govmomi/clustermodules"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/session"
)

type service struct{}

func NewService() Service {
	return service{}
}

func (s service) Create(ctx *context.ClusterContext, wrapper Wrapper) (string, error) {
	logger := ctx.Logger.WithValues("object", wrapper.GetName())
	template, err := fetchMachineTemplate(ctx, wrapper)
	if err != nil {
		logger.V(4).Error(err, "error fetching template")
		return "", err
	}

	vCenterSession, err := fetchSessionForObject(ctx, template)
	if err != nil {
		logger.V(4).Error(err, "error fetching session")
		return "", err
	}

	// Fetch the compute cluster resource by tracing the owner of the resource pool in use.
	// TODO (srm09): How do we support Multi AZ scenarios here
	computeClusterRef, err := getComputeClusterResource(ctx, vCenterSession, template.Spec.Template.Spec.ResourcePool)
	if err != nil {
		logger.V(4).Error(err, "error fetching compute cluster resource")
		return "", err
	}

	provider := clustermodules.NewProvider(vCenterSession.TagManager.Client)
	moduleUUID, err := provider.CreateModule(ctx, computeClusterRef)
	if err != nil {
		logger.V(4).Error(err, "error creating cluster module")
		return "", err
	}
	logger.V(4).Info("created cluster module for object", "moduleUUID", moduleUUID)
	return moduleUUID, nil
}

func (s service) DoesExist(ctx *context.ClusterContext, wrapper Wrapper, moduleUUID string) (bool, error) {
	logger := ctx.Logger.WithValues("object", wrapper.GetName())
	template, err := fetchMachineTemplate(ctx, wrapper)
	if err != nil {
		logger.V(4).Error(err, "error fetching template")
		return false, err
	}

	vCenterSession, err := fetchSessionForObject(ctx, template)
	if err != nil {
		logger.V(4).Error(err, "error fetching session")
		return false, err
	}

	// Fetch the compute cluster resource by tracing the owner of the resource pool in use.
	// TODO (srm09): How do we support Multi AZ scenarios here
	computeClusterRef, err := getComputeClusterResource(ctx, vCenterSession, template.Spec.Template.Spec.ResourcePool)
	if err != nil {
		logger.V(4).Error(err, "error fetching compute cluster resource")
		return false, err
	}

	provider := clustermodules.NewProvider(vCenterSession.TagManager.Client)
	return provider.DoesModuleExist(ctx, moduleUUID, computeClusterRef)
}

func (s service) Remove(ctx *context.ClusterContext, moduleUUID string) error {
	params := newParams(*ctx)
	vcenterSession, err := fetchSession(ctx, params)
	if err != nil {
		return err
	}

	provider := clustermodules.NewProvider(vcenterSession.TagManager.Client)
	if err := provider.DeleteModule(ctx, moduleUUID); err != nil {
		return err
	}
	return nil
}

func getComputeClusterResource(ctx goctx.Context, s *session.Session, resourcePool string) (types.ManagedObjectReference, error) {
	rp, err := s.Finder.ResourcePoolOrDefault(ctx, resourcePool)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}

	cc, err := rp.Owner(ctx)
	if err != nil {
		return types.ManagedObjectReference{}, err
	}

	ownerPath, err := find.InventoryPath(ctx, s.Client.Client, cc.Reference())
	if err != nil {
		return types.ManagedObjectReference{}, err
	}
	if _, err = s.Finder.ClusterComputeResource(ctx, ownerPath); err != nil {
		return types.ManagedObjectReference{}, IncompatibleOwnerError{cc.Reference().Value}
	}

	return cc.Reference(), nil
}
