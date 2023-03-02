// (C) Copyright Confidential Containers Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/confidential-containers/cloud-api-adaptor/test/provutils"
)

func main() {
	prov, _ := provutils.GetCloudProvisioner()

	action := flag.String("action", "provision", "string")
	flag.Parse()

	if *action == "provision" {
		if err := prov.CreateVPC(context.TODO(), nil); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := prov.CreateCluster(context.TODO(), nil); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if *action == "deprovision" {
		if err := prov.DeleteCluster(context.TODO(), nil); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if err := prov.DeleteVPC(context.TODO(), nil); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

	}

	if *action == "uploadimage" {
		podvmImage := os.Getenv("TEST_E2E_PODVM_IMAGE")
		if _, err := os.Stat(podvmImage); os.IsNotExist(err) {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := prov.UploadPodvm(podvmImage, context.TODO(), nil); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}
