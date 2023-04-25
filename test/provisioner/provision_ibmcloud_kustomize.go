//go:build ibmcloud

// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package provisioner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func init() {
	newInstallOverlayFunctions["ibmcloud"] = NewIBMCloudInstallOverlay
}

// IBMCloudInstallOverlay implements the InstallOverlay interface
type IBMCloudInstallOverlay struct {
	overlay *KustomizeOverlay
}

type QuayTagsResponse struct {
	Tags []struct {
		Name       string `json:"name"`
		StartTime  string `json:"start_ts"`
		ModifiedAt string `json:"last_modified"`
		Digest     string `json:"manifest_digest"`
		Size       string `json:"size"`
		Manifest   bool   `json:"is_manifest_list"`
		Reversion  bool   `json:"reversion"`
	} `json:"tags"`
	Others map[string]interface{} `json:"-"`
}

func isKustomizeConfigMapKey(key string) bool {
	switch key {
	case "CLOUD_PROVIDER":
		return true
	case "IBMCLOUD_VPC_ENDPOINT":
		return true
	case "IBMCLOUD_RESOURCE_GROUP_ID":
		return true
	case "IBMCLOUD_SSH_KEY_ID":
		return true
	case "IBMCLOUD_PODVM_IMAGE_ID":
		return true
	case "IBMCLOUD_PODVM_INSTANCE_PROFILE_NAME":
		return true
	case "IBMCLOUD_ZONE":
		return true
	case "IBMCLOUD_VPC_SUBNET_ID":
		return true
	case "IBMCLOUD_VPC_SG_ID":
		return true
	case "IBMCLOUD_VPC_ID":
		return true
	case "CRI_RUNTIME_ENDPOINT":
		return true
	default:
		return false
	}
}

func isKustomizeSecretKey(key string) bool {
	switch key {
	case "IBMCLOUD_API_KEY":
		return true
	case "IBMCLOUD_IAM_ENDPOINT":
		return true
	case "IBMCLOUD_ZONE":
		return true
	default:
		return false
	}
}

func getCaaNewTagFromCommit() string {
	resp, err := http.Get("https://quay.io/api/v1/repository/confidential-containers/cloud-api-adaptor/tag/")
	if err != nil {
		log.Errorf(err.Error())
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf(err.Error())
	}

	var result QuayTagsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Errorf(err.Error())
		return ""
	}

	for _, tag := range result.Tags {
		if tag.Manifest && len(tag.Name) == 40 { // the latest git commit hash tag
			return tag.Name
		}
	}

	return ""
}

func NewIBMCloudInstallOverlay() (InstallOverlay, error) {
	overlay, err := NewKustomizeOverlay("../../install/overlays/ibmcloud")
	if err != nil {
		return nil, err
	}

	return &IBMCloudInstallOverlay{
		overlay: overlay,
	}, nil
}

func (lio *IBMCloudInstallOverlay) Apply(ctx context.Context, cfg *envconf.Config) error {
	return lio.overlay.Apply(ctx, cfg)
}

func (lio *IBMCloudInstallOverlay) Delete(ctx context.Context, cfg *envconf.Config) error {
	return lio.overlay.Delete(ctx, cfg)
}

// Update install/overlays/ibmcloud/kustomization.yaml
func (lio *IBMCloudInstallOverlay) Edit(ctx context.Context, cfg *envconf.Config, properties map[string]string) error {
	log.Debugf("%+v", properties)
	var err error

	// image
	var newTag string
	if IBMCloudProps.CaaImageTag != "" {
		newTag = IBMCloudProps.CaaImageTag
	} else {
		newTag = getCaaNewTagFromCommit()
	}
	if newTag != "" {
		log.Infof("Updating caa image tag with %s", newTag)
		if err = lio.overlay.SetKustomizeImage("cloud-api-adaptor", "newTag", newTag); err != nil {
			return err
		}
	}

	for k, v := range properties {
		// configMapGenerator
		if isKustomizeConfigMapKey(k) {
			if err = lio.overlay.SetKustomizeConfigMapGeneratorLiteral("peer-pods-cm", k, v); err != nil {
				return err
			}
		}
		// secretGenerator
		if isKustomizeSecretKey(k) {
			if err = lio.overlay.SetKustomizeSecretGeneratorLiteral("peer-pods-secret", k, v); err != nil {
				return err
			}
		}
	}

	if err = lio.overlay.YamlReload(); err != nil {
		return err
	}

	return nil
}
