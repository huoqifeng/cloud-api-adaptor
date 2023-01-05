//go:build ibmcloud
package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	kconf "sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// IBMCloudProvisioner implements the CloudProvision interface for Libvirt.
type IBMCloudProvisioner struct {
}

func NewIBMCloudProvisioner() (*IBMCloudProvisioner, error) {

	return &IBMCloudProvisioner{}, nil
}

func (c *IBMCloudProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	// example: ./create_iks_cluster.sh name="stvncluster" apikey=${myAPIKey} region="us-south" flavor="bx2.2x8" resource_group="iks2022"
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(cwd, "../../ibmcloud")
	fmt.Println("path: ", path)
	cmd := exec.Command("/bin/bash", "./create_iks_cluster.sh",
			fmt.Sprintf("name=%s", os.Getenv("CLUSTER_NAME")),
			fmt.Sprintf("apikey=%s", os.Getenv("APIKEY")),
			fmt.Sprintf("region=%s", os.Getenv("REGION")),
			fmt.Sprintf("zone=%s", os.Getenv("ZONE")),
			fmt.Sprintf("flavor=%s", os.Getenv("FLAVOR")),
			fmt.Sprintf("resource_group=%s", os.Getenv("RESOURCE_GROUP")),
			fmt.Sprintf("version=%s", os.Getenv("IKS_VERSION")),
			fmt.Sprintf("workers=%s", os.Getenv("WORKERS")))
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	fmt.Println("out: ", string(out))
	if err != nil {
		return err
	}

	// Look for a suitable kubeconfig file in the sequence: --kubeconfig flag,
	// or KUBECONFIG variable, or $HOME/.kube/config.
	kubeconfig := kconf.ResolveKubeConfigFile()
	if kubeconfig == "" {
		return errors.New("Unabled to find a kubeconfig file")
	}
	cfg.WithKubeconfigFile(kubeconfig)

	return nil
}

func (c *IBMCloudProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (c *IBMCloudProvisioner) DeleteVPC(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (c *IBMCloudProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	// example: ./delete_iks_cluster.sh name="stvncluster" apikey=${myAPIKey} region="us-south"
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	path := filepath.Join(cwd, "../../ibmcloud")
	fmt.Println("path: ", path)
	cmd := exec.Command("/bin/bash", "./delete_iks_cluster.sh",
			fmt.Sprintf("name=%s", os.Getenv("CLUSTER_NAME")),
			fmt.Sprintf("apikey=%s", os.Getenv("APIKEY")),
			fmt.Sprintf("region=%s", os.Getenv("REGION")))
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	fmt.Println("out: ", string(out))
	if err != nil {
		return err
	}

	return nil
}

func GetCloudProvisioner() (CloudProvision, error) {
	return NewIBMCloudProvisioner()
}
