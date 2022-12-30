//go:build ibmcloud

package e2e

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/vpc-go-sdk/vpcv1"
)

var vpc *vpcv1.VpcV1

func initVpcV1() (*vpcv1.VpcV1, error) {
	if vpc != nil {
		return vpc, nil
	}
	apiKey := os.Getenv("APIKEY")
	iamServiceURL := os.Getenv("IAM_SERVICE_URL")
	vpcServiceURL := os.Getenv("VPC_SERVICE_URL")
	if len(apiKey) <= 0 {
		return nil, errors.New("APIKEY was not set.")
	}
	if len(iamServiceURL) <= 0 {
		return nil, errors.New("IAM_SERVICE_URL was not set.")
	}
	if len(vpcServiceURL) <= 0 {
		return nil, errors.New("VPC_SERVICE_URL was not set.")
	}
	vpcv1, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: &core.IamAuthenticator{
			ApiKey: apiKey,
			URL:    iamServiceURL,
		},
		URL: vpcServiceURL,
	})
	if err != nil {
		return nil, err
	}
	vpc = vpcv1
	return vpc, nil
}

func TestCreateSimplePod(t *testing.T) {
	vpc, err := initVpcV1()
	if err != nil {
		t.Fatal(err)
	}
	assert := IBMCloudAssert{
		vpc,
	}
	doTestCreateSimplePod(t, assert)
}

// IBMCloudAssert implements the CloudAssert interface for IBMCloud.
type IBMCloudAssert struct {
	vpc *vpcv1.VpcV1
}

func (c IBMCloudAssert) HasPodVM(t *testing.T, id string) {
	fmt.Println("PodVM name: ", id)
	options := &vpcv1.ListInstancesOptions{}
	instances, _, err := c.vpc.ListInstances(options)

	if err != nil {
		t.Fatal(err)
	}

	for i, instance := range instances.Instances {
		name := *instance.Name
		fmt.Println("Instance number: ", i, " Instance id: ", *instance.ID, " Instance name: ", name)
		// TODO: PodVM name is podvm-POD_NAME-SANDBOX_ID, where SANDBOX_ID is truncated
		// in the 8th word. Ideally we should match the exact name, not just podvm-POD_NAME.
		if strings.HasPrefix(name, strings.Join([]string{"podvm", id, ""}, "-")) {
			return
		}
	}

	// It didn't find the PodVM if it reached here.
	t.Error("PodVM was not created")
}
