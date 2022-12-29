//go:build ibmcloud
package e2e

import (
	"context"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// IBMCloudProvisioner implements the CloudProvision interface for Libvirt.
type IBMCloudProvisioner struct {
}

func NewIBMCloudProvisioner(network string, storage string) (*IBMCloudProvisioner, error) {
	return &IBMCloudProvisioner{}, nil
}

func (l *IBMCloudProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (l *IBMCloudProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func GetCloudProvisioner() (CloudProvision, error) {
	return NewIBMCloudProvisioner("default", "default")
}
