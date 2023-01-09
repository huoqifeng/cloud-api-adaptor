#!/bin/bash

set -o errexit

printHelp(){

cat << CMD
   Usage: $0 <cmd> [<args>]

   args:
      -name                      The name of the cluster which you want to create.
      -profile                   The profile of the new control plane/worker. Such as 'bx2.2x8'
      -region                    The region of the new control plane/worker. Such as 'us-south'.
      -zone                      The zone of the new control plane/worker. Such as 'us-south-1'.
      -apikey                    API Keys for your account in IBM Cloud.
      -sshkey                    SSH Key name to debug the PeerPod
      -image                     PeerPod VM Image Name

   Sample Usage: 
   
      ./customize_operator_overlay.sh name="testcluster" apikey=${myAPIKey} region="us-south" zone="us-south-1" profile="bx2-2x8" sshkey="testsshkey" image="e2e-pod-image-amd64"

CMD

   exit 0

}


echo "args:"$@

if [[ -z $@ ]]; then
   printHelp
fi

for arg in "$@"
do
  key=$(echo $arg | cut -f1 -d=)
  value=$(echo $arg| cut -f2 -d=)
  case "$key" in
     name)
        cluster_name=$value
        ;;
     apikey)
        apikey=$value
        ;;
     region)
        region=$value
        ;;
     zone)
        zone=$value
        ;;    
     profile)
        podvm_instance_profile_name=$value
        ;;
     sshkey)
        ssh_key_name=$value
        ;;
     image)
        podvm_image_name=$value
        ;;
      -help)
         printHelp
         ;;
      --help)
         printHelp
         ;;
     *)
        echo "Ignoring unexpected arg $arg"
   esac
done

if [[ -z $cluster_name ]]; then
    echo 'Please set name arg to the name of the Cluster you want to create.'
    exit 99
fi

if [[ -z $apikey ]]; then
    echo 'Please set apikey for your IBM Cloud account.'
    exit 99
fi

if [[ -z $region ]]; then
    echo 'region not input use default: us-south'
    region='us-south'
fi
if [[ -z $zone ]]; then
    echo 'zone not input use default: us-south-2'
    zone='us-south-2'
fi
if [[ -z $podvm_instance_profile_name ]]; then
    echo 'profile not input use default: bx2.2x8'
    podvm_instance_profile_name='bx2-2x8'
fi
if [[ -z $podvm_image_name ]]; then
    echo 'pod vm image is not specified'
    exit 99
fi

vpc_name=${cluster_name}-vpc
subnet_name=${cluster_name}-sn1

# configure kustomization.yaml
target_file_path="../install/overlays/ibmcloud/kustomization.yaml"
resource_group_id=$(ibmcloud resource groups --output JSON | jq -r '.[] | select (.default == true) .id')
ssh_key_id=$(ibmcloud is keys --all-resource-groups --output JSON | jq -r '.[] | select (.name == "'"${ssh_key_name}"'") .id')
subnet_id=$(ibmcloud is subnet "${subnet_name}" --output JSON | jq -r '.id')
default_security_group_id=$(ibmcloud is vpc "${vpc_name}" --output JSON | jq -r .default_security_group.id)
vpc_id=$(ibmcloud is vpc "${vpc_name}" --output JSON | jq -r '.id')
podvm_image_id=$(ibmcloud is image "${podvm_image_name}" --output JSON | jq -r '.id')

sed -i 's/IBMCLOUD_VPC_ENDPOINT=.*/IBMCLOUD_VPC_ENDPOINT="https:\/\/'"${region}"'.iaas.cloud.ibm.com\/v1"/' ${target_file_path}
sed -i 's/IBMCLOUD_RESOURCE_GROUP_ID=.*/IBMCLOUD_RESOURCE_GROUP_ID="'"${resource_group_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_SSH_KEY_ID=.*/IBMCLOUD_SSH_KEY_ID="'"${ssh_key_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_PODVM_IMAGE_ID=.*/IBMCLOUD_PODVM_IMAGE_ID="'"${podvm_image_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_PODVM_INSTANCE_PROFILE_NAME=.*/IBMCLOUD_PODVM_INSTANCE_PROFILE_NAME="'"${podvm_instance_profile_name}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_ZONE=.*/IBMCLOUD_ZONE="'"${zone}"'"/g' ${target_file_path}
sed -i 's/IBMCLOUD_VPC_SUBNET_ID=.*/IBMCLOUD_VPC_SUBNET_ID="'"${subnet_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_VPC_SG_ID=.*/IBMCLOUD_VPC_SG_ID="'"${default_security_group_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_VPC_ID=.*/IBMCLOUD_VPC_ID="'"${vpc_id}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_API_KEY=.*/IBMCLOUD_API_KEY="'"${apikey}"'"/' ${target_file_path}
sed -i 's/IBMCLOUD_IAM_ENDPOINT=.*/IBMCLOUD_IAM_ENDPOINT="https:\/\/iam.cloud.ibm.com\/identity\/token"/' ${target_file_path}

echo "Overlay yaml is customized."
kubectl apply -f ../install/yamls/deploy.yaml
kubectl apply -k ../install/overlays/ibmcloud
echo "PeerPod operator deploying"

# Wait RuntimeClass kata is created
waitRuntimeClassReady() {
    echo "Get RuntimeClass kata"
    CHECK_ATTEMPTS=$1
    MAX_ATTEMPTS_ALLOWED=30
    WAIT_BETWEEN_STATUS_CHECK=10

    kubectl get po -n confidential-containers-system
    kata_name=$(kubectl get runtimeclass -o json | jq -r '.items[] | select (.metadata.name == "kata") .metadata .name')

    if [[ -z ${kata_name} ]]; then
        echo "${CHECK_ATTEMPTS} attempts of ${MAX_ATTEMPTS_ALLOWED} attempts used."

        if [[ $CHECK_ATTEMPTS -lt $MAX_ATTEMPTS_ALLOWED ]]; then
            echo "Waiting ${WAIT_BETWEEN_STATUS_CHECK} seconds before re-checking runtime class"
            CHECK_ATTEMPTS=$((CHECK_ATTEMPTS + 1))
            echo "CHECK_ATTEMPTS:"$CHECK_ATTEMPTS
            sleep ${WAIT_BETWEEN_STATUS_CHECK}
            waitRuntimeClassReady ${CHECK_ATTEMPTS}
        else
            echo "Failed to get RuntimeClass kata after $MAX_ATTEMPTS_ALLOWED attempts with $WAIT_BETWEEN_STATUS_CHECK seconds waiting between each check."
            return 99
        fi
    else
        echo "RuntimeClass kata is created in cluster."
        return 0
    fi
}

waitRuntimeClassReady 0
echo "PeerPod operator deployed"