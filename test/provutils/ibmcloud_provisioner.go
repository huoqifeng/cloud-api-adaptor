//go:build ibmcloud

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package provutils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"strconv"
	"strings"
	"sync"

	bx "github.com/IBM-Cloud/bluemix-go"
	"github.com/IBM-Cloud/bluemix-go/api/container/containerv2"
	bxsession "github.com/IBM-Cloud/bluemix-go/session"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/ibm-cos-sdk-go/aws"
	"github.com/IBM/ibm-cos-sdk-go/aws/credentials/ibmiam"
	cosession "github.com/IBM/ibm-cos-sdk-go/aws/session"
	"github.com/IBM/ibm-cos-sdk-go/service/s3/s3manager"
	"github.com/IBM/vpc-go-sdk/vpcv1"
)

var (
	VPC             *vpcv1.VpcV1
	ClusterAPI      containerv2.Clusters
	ResourceGroupID string
	ClusterName     string
	VpcName         string
	VpcID           string
	DefaultSGID     string
	Region          string
	Zone            string
	SubnetName      string
	SubnetID        string
	PodvmImageID    string
	PodvmImageArch  string
	IksVersion      string
	SshKeyID        string
	ApiKey          string
	IamServiceURL   string
	VpcServiceURL   string
	WorkerFlavor    string
	IsSelfManaged   string
	CosInstanceID   string
	Bucket          string
	CosServiceURL   string
	InstanceProfile string
	WorkerCount     int
)

func init() {
	// TODO use prop files rather than system variables
	ApiKey = os.Getenv("APIKEY")
	IamServiceURL = os.Getenv("IAM_SERVICE_URL")
	VpcServiceURL = os.Getenv("VPC_SERVICE_URL")
	ResourceGroupID = os.Getenv("RESOURCE_GROUP_ID")
	ClusterName = os.Getenv("CLUSTER_NAME")
	Region = os.Getenv("REGION")
	Zone = os.Getenv("ZONE")
	SshKeyID = os.Getenv("SSH_KEY_ID")
	IsSelfManaged = os.Getenv("IS_SELF_MANAGED_CLUSTER")
	CosInstanceID = os.Getenv("COS_INSTANCE_ID")
	Bucket = os.Getenv("COS_BUCKET")
	CosServiceURL = os.Getenv("COS_SERVICE_URL")
	PodvmImageArch = os.Getenv("PODVM_IMAGE_ARCH")
	InstanceProfile = os.Getenv("INSTANCE_PROFILE_NAME")
	workerCountStr := os.Getenv("WORKERS")

	if len(ApiKey) <= 0 {
		panic(fmt.Errorf("APIKEY was not set."))
	}
	if len(IamServiceURL) <= 0 {
		panic(fmt.Errorf("IAM_SERVICE_URL was not set, example: https://iam.cloud.ibm.com/identity/token"))
	}
	if len(VpcServiceURL) <= 0 {
		panic(fmt.Errorf("VPC_SERVICE_URL was not set, example: https://us-south.iaas.cloud.ibm.com/v1"))
	}
	if len(ResourceGroupID) <= 0 {
		panic(fmt.Errorf("RESOURCE_GROUP_ID was not set."))
	}
	if len(Region) <= 0 {
		panic(fmt.Errorf("REGION was not set."))
	}
	if len(Zone) <= 0 {
		panic(fmt.Errorf("ZONE was not set."))
	}
	if len(CosInstanceID) <= 0 {
		panic(fmt.Errorf("COS_INSTANCE_ID was not set."))
	}
	if len(Bucket) <= 0 {
		panic(fmt.Errorf("COS_BUCKET was not set."))
	}
	if len(CosServiceURL) <= 0 {
		panic(fmt.Errorf("COS_SERVICE_URL was not set, example: s3.us.cloud-object-storage.appdomain.cloud"))
	}

	if len(workerCountStr) <= 0 {
		WorkerCount = 1
	} else {
		count, err := strconv.Atoi(workerCountStr)
		if err != nil {
			WorkerCount = 1
		} else {
			WorkerCount = count
		}
	}
	if len(ClusterName) <= 0 {
		ClusterName = "e2e_test_cluster"
	}

	VpcName = ClusterName + "_vpc"
	SubnetName = ClusterName + "_subnet"

	err := initVpcV1()
	if err != nil {
		panic(err)
	}
	err = initClustersAPI()
	if err != nil {
		panic(err)
	}
}

func initVpcV1() error {
	if VPC != nil {
		return nil
	}

	vpcService, err := vpcv1.NewVpcV1(&vpcv1.VpcV1Options{
		Authenticator: &core.IamAuthenticator{
			ApiKey: ApiKey,
			URL:    IamServiceURL,
		},
		URL: VpcServiceURL,
	})
	if err != nil {
		return err
	}
	VPC = vpcService
	return nil
}

func initClustersAPI() error {
	cfg := &bx.Config{
		BluemixAPIKey: ApiKey,
		Region:        Region,
	}
	sess, err := bxsession.New(cfg)
	if err != nil {
		return err
	}
	clusterClient, err := containerv2.New(sess)
	if err != nil {
		return err
	}
	ClusterAPI = clusterClient.Clusters()
	return nil
}

// CustomReader helps uplaod cos object
type CustomReader struct {
	fp      *os.File
	size    int64
	read    int64
	signMap map[int64]struct{}
	mux     sync.Mutex
}

func (r *CustomReader) Read(p []byte) (int, error) {
	return r.fp.Read(p)
}

func (r *CustomReader) ReadAt(p []byte, off int64) (int, error) {
	n, err := r.fp.ReadAt(p, off)
	if err != nil {
		return n, err
	}

	r.mux.Lock()
	// Ignore the first signature call
	if _, ok := r.signMap[off]; ok {
		// Got the length have read( or means has uploaded), and you can construct your message
		r.read += int64(n)
		fmt.Printf("\rtotal read:%d    progress:%d%%", r.read, int(float32(r.read*100)/float32(r.size)))
	} else {
		r.signMap[off] = struct{}{}
	}
	r.mux.Unlock()
	return n, err
}

func (r *CustomReader) Seek(offset int64, whence int) (int64, error) {
	return r.fp.Seek(offset, whence)
}

func createVPC() error {
	classicAccess := false
	manual := "manual"

	options := &vpcv1.CreateVPCOptions{
		ResourceGroup: &vpcv1.ResourceGroupIdentity{
			ID: &ResourceGroupID,
		},
		Name:                    &[]string{VpcName}[0],
		ClassicAccess:           &classicAccess,
		AddressPrefixManagement: &manual,
	}
	vpcInstance, _, err := VPC.CreateVPC(options)
	if err != nil {
		return err
	}
	VpcID = *vpcInstance.ID

	if len(VpcID) <= 0 {
		return errors.New("VpcID is empty, unknown error happened when create VPC.")
	}

	sgoptions := &vpcv1.GetVPCDefaultSecurityGroupOptions{}
	sgoptions.SetID(VpcID)
	defaultSG, _, err := VPC.GetVPCDefaultSecurityGroup(sgoptions)
	if err != nil {
		return err
	}

	DefaultSGID = *defaultSG.ID

	return nil
}

func deleteVPC() error {
	// TODO, check vpc existing
	deleteVpcOptions := &vpcv1.DeleteVPCOptions{}
	deleteVpcOptions.SetID(VpcID)
	_, err := VPC.DeleteVPC(deleteVpcOptions)

	if err != nil {
		return err
	}
	return nil
}

func createSubnet() error {
	cidrBlock := "10.0.1.0/24"
	options := &vpcv1.CreateSubnetOptions{}
	options.SetSubnetPrototype(&vpcv1.SubnetPrototype{
		Ipv4CIDRBlock: &cidrBlock,
		Name:          &[]string{SubnetName}[0],
		VPC: &vpcv1.VPCIdentity{
			ID: &VpcID,
		},
		Zone: &vpcv1.ZoneIdentity{
			Name: &Zone,
		},
	})
	subnet, _, err := VPC.CreateSubnet(options)
	if err != nil {
		return err
	}
	SubnetID = *subnet.ID

	if len(SubnetID) <= 0 {
		return errors.New("SubnetID is empty, unknown error happened when create Subnet.")
	}

	return nil
}

func deleteSubnet() error {
	// TODO, check vpc existing
	options := &vpcv1.DeleteSubnetOptions{}
	options.SetID(SubnetID)
	_, err := VPC.DeleteSubnet(options)

	if err != nil {
		return err
	}
	return nil
}

func createVpcImpl() error {
	err := createVPC()
	if err != nil {
		return err
	}
	return createSubnet()
}

func deleteVpcImpl() error {
	err := deleteSubnet()
	if err != nil {
		return err
	}
	return deleteVPC()
}

// IBMCloudProvisioner implements the CloudProvision interface for ibmcloud.
type IBMCloudProvisioner struct {
}

// IBMCloudProvisioner

func (p *IBMCloudProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	clusterInfo := containerv2.ClusterCreateRequest{
		DisablePublicServiceEndpoint: true,
		Name:                         ClusterName,
		Provider:                     "vpc-gen2",
		WorkerPools: containerv2.WorkerPoolConfig{
			CommonWorkerPoolConfig: containerv2.CommonWorkerPoolConfig{
				DiskEncryption: true,
				Flavor:         "bx2.2x8",
				VpcID:          VpcID,
				WorkerCount:    WorkerCount,
				Zones: []containerv2.Zone{
					{
						ID:       Zone,
						SubnetID: SubnetID,
					},
				},
			},
		},
	}
	target := containerv2.ClusterTargetHeader{}
	_, err := ClusterAPI.Create(clusterInfo, target)
	if err != nil {
		return err
	}

	return nil
}

func (p *IBMCloudProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return createVpcImpl()
}

func (p *IBMCloudProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	target := containerv2.ClusterTargetHeader{}
	err := ClusterAPI.Delete(ClusterName, target)
	if err != nil {
		return err
	}

	return nil
}

func (p *IBMCloudProvisioner) DeleteVPC(ctx context.Context, cfg *envconf.Config) error {
	return deleteVpcImpl()
}

func (p *IBMCloudProvisioner) UploadPodvm(imagePath string, ctx context.Context, cfg *envconf.Config) error {
	filePath, err := filepath.Abs(imagePath)
	if err != nil {
		return err
	}

	conf := aws.NewConfig().
		WithEndpoint(CosServiceURL).
		WithCredentials(ibmiam.NewStaticCredentials(aws.NewConfig(),
			IamServiceURL, ApiKey, CosInstanceID)).
		WithS3ForcePathStyle(true)

	sess := cosession.Must(cosession.NewSession(conf))

	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	reader := &CustomReader{
		fp:      file,
		size:    fileInfo.Size(),
		signMap: map[int64]struct{}{},
	}

	uploader := s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024
		u.LeavePartsOnError = true
	})

	key := filepath.Base(filePath)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(Bucket),
		Key:    aws.String(key),
		Body:   reader,
	})
	if err != nil {
		return err
	}

	var osNames []string
	if strings.EqualFold("s390x", PodvmImageArch) {
		osNames = []string{"ubuntu-20-04-s390x"}
	} else {
		osNames = []string{"ubuntu-20-04-amd64"}

	}
	operatingSystemIdentityModel := &vpcv1.OperatingSystemIdentityByName{
		Name: &osNames[0],
	}

	cosID := "cos://" + Region + "/" + Bucket + "/" + key
	imageName := key
	options := &vpcv1.CreateImageOptions{}
	options.SetImagePrototype(&vpcv1.ImagePrototype{
		Name: &imageName,
		File: &vpcv1.ImageFilePrototype{
			Href: &cosID,
		},
		OperatingSystem: operatingSystemIdentityModel,
	})
	image, _, err := VPC.CreateImage(options)
	if err != nil {
		return err
	}
	PodvmImageID = *image.ID

	return nil
}

func (p *IBMCloudProvisioner) DoKustomize(ctx context.Context, cfg *envconf.Config) error {
	overlayFile := "../../install/overlays/ibmcloud/kustomization.yaml"
	overlayFileBak := "../../install/overlays/ibmcloud/kustomization.yaml.bak"
	err := os.Rename(overlayFile, overlayFileBak)
	if err != nil {
		return err
	}

	input, err := os.ReadFile(overlayFileBak)
	if err != nil {
		return err
	}

	replacer := strings.NewReplacer("IBMCLOUD_VPC_ENDPOINT=\"\"", "IBMCLOUD_VPC_ENDPOINT=\""+VpcServiceURL+"\"",
		"IBMCLOUD_RESOURCE_GROUP_ID=\"\"", "IBMCLOUD_RESOURCE_GROUP_ID=\""+ResourceGroupID+"\"",
		"IBMCLOUD_SSH_KEY_ID=\"\"", "IBMCLOUD_SSH_KEY_ID=\""+SshKeyID+"\"",
		"IBMCLOUD_PODVM_IMAGE_ID=\"\"", "IBMCLOUD_PODVM_IMAGE_ID=\""+PodvmImageID+"\"",
		"IBMCLOUD_PODVM_INSTANCE_PROFILE_NAME=\"\"", "IBMCLOUD_PODVM_INSTANCE_PROFILE_NAME=\""+InstanceProfile+"\"",
		"IBMCLOUD_ZONE=\"\"", "IBMCLOUD_ZONE=\""+Zone+"\"",
		"IBMCLOUD_VPC_SUBNET_ID=\"\"", "IBMCLOUD_VPC_SUBNET_ID=\""+SubnetID+"\"",
		"IBMCLOUD_VPC_SG_ID=\"\"", "IBMCLOUD_VPC_SG_ID=\""+DefaultSGID+"\"",
		"IBMCLOUD_VPC_ID=\"\"", "IBMCLOUD_VPC_ID=\""+VpcID+"\"",
		"IBMCLOUD_API_KEY=\"\"", "IBMCLOUD_API_KEY=\""+ApiKey+"\"",
		"IBMCLOUD_IAM_ENDPOINT=\"\"", "IBMCLOUD_IAM_ENDPOINT=\""+IamServiceURL+"\"")

	output := replacer.Replace(string(input))

	if err = os.WriteFile(overlayFile, []byte(output), 0666); err != nil {
		return err
	}

	return nil
}

func (p *IBMCloudProvisioner) GetVPCDefaultSecurityGroupID(vpcID string) (string, error) {
	options := &vpcv1.GetVPCDefaultSecurityGroupOptions{}
	options.SetID(vpcID)
	defaultSG, _, err := VPC.GetVPCDefaultSecurityGroup(options)
	if err != nil {
		return "", err
	}

	return *defaultSG.ID, nil
}

// SelfManagedClusterProvisioner implements the CloudProvision interface for self-managed k8s cluster in ibmcloud VSI.
type SelfManagedClusterProvisioner struct {
}

// SelfManagedClusterProvisioner

func (p *SelfManagedClusterProvisioner) CreateCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SelfManagedClusterProvisioner) CreateVPC(ctx context.Context, cfg *envconf.Config) error {
	return createVpcImpl()
}

func (p *SelfManagedClusterProvisioner) DeleteCluster(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SelfManagedClusterProvisioner) DeleteVPC(ctx context.Context, cfg *envconf.Config) error {
	return deleteVpcImpl()
}

func (p *SelfManagedClusterProvisioner) UploadPodvm(imagePath string, ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func (p *SelfManagedClusterProvisioner) DoKustomize(ctx context.Context, cfg *envconf.Config) error {
	return nil
}

func getIBMCloudProvisioner() (CloudProvision, error) {
	if IsSelfManaged == "yes" {
		return &SelfManagedClusterProvisioner{}, nil
	} else {
		return &IBMCloudProvisioner{}, nil
	}
}
