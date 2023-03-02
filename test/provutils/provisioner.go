// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package provutils

import (
	"context"
	"os"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// CloudProvision defines operations to provision the environment on cloud providers.
type CloudProvision interface {
	CreateCluster(ctx context.Context, cfg *envconf.Config) error
	CreateVPC(ctx context.Context, cfg *envconf.Config) error
	DeleteCluster(ctx context.Context, cfg *envconf.Config) error
	DeleteVPC(ctx context.Context, cfg *envconf.Config) error
	UploadPodvm(imagePath string, ctx context.Context, cfg *envconf.Config) error
	DoKustomize(ctx context.Context, cfg *envconf.Config) error
}

// SelfManagedClusterProvisioner implements the CloudProvision interface for self-managed k8s cluster in ibmcloud VSI.
type SampleProvisioner struct {
}

// SelfManagedClusterProvisioner

func (p *SampleProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SampleProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SampleProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SampleProvisioner) DeleteVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SampleProvisioner) UploadPodvm(imagePath string, ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SampleProvisioner) DoKustomize(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

//nolint:typecheck
func GetCloudProvisioner() (CloudProvision, error) {
	provider := os.Getenv("CLOUD_PROVIDER")

	if provider == "libvirt" {
		return getLibvirtCloudProvisioner()
	}

	if provider == "ibmcloud" {
		return getIBMCloudProvisioner()

	}

	return &SampleProvisioner{}, nil
}
