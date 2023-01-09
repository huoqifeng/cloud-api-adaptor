#!/bin/bash

# requires jq, ibmcloud cli, cloud-object-storage, infrastructure-service, container-service plugin and Kubernetes command line tool
# following https://cloud.ibm.com/docs/cli?topic=cli-install-ibmcloud-cli
#           https://cloud.ibm.com/docs/cli?topic=cli-install-devtools-manually
# example:
#   apt-get install jq
#   curl -fsSL https://clis.cloud.ibm.com/install/linux | sh
#   ibmcloud plugin install container-service -f
#   ibmcloud plugin install infrastructure-service -f
#   ibmcloud plugin install cloud-object-storage -f
#   curl --progress-bar -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl

set -o errexit

printHelp(){

cat << CMD
   Usage: $0 <cmd> [<args>]

   args:

      -name                      The name of the cluster which you want to create.
      -flavor                    The profile of the new control plane/worker. Such as 'bx2.2x8'
      -region                    The zone of the new control plane/worker. Such as 'us-south-1'.
      -version                   IKS version. Such as '1.23.5'
      -workers                   Worker node count, default: 1.
      -apikey                    API Keys for your account in IBM Cloud.
      -resource_group            The resource group name. 

   Sample Usage: 
   
      ./create_iks_cluster.sh name="testcluster" apikey=${myAPIKey} region="us-south" flavor="bx2.2x8" resource_group="iks2022"

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
     version)
        version=$value
        ;;
     flavor)
        flavor=$value
        ;;
     workers)
        workers=$value
        ;;
     resource_group)
        resource_group=$value
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
if [[ -z $flavor ]]; then
    echo 'flavor not input use default: bx2.2x8'
    flavor='bx2.2x8'
fi
if [[ -z $workers ]]; then
    echo 'workers count not input use default: 1'
    workers=1
fi

echo "production environment"
export login_endpoint="cloud.ibm.com"
export iks_endpoint="https://containers.cloud.ibm.com"

# Login to test.cloud, use us-south and a named resource group
ibmcloud login -a ${login_endpoint} -r ${region} --apikey ${apikey}


checkAccount() {
   account_type=$(ibmcloud account show --output JSON|jq -r '.type')
   if [[ $account_type == "TRIAL" ]]; then
      echo "Your account is TRIAL which can't create cluster. Pls upgrade it to paid account. You may refer to https://test.cloud.ibm.com/docs/get-coding?topic=get-coding-test-account"
      exit 99
   fi
}

checkAccount

if [[ -z $resource_group ]]; then
   echo 'Arg of resource_group is not set. default resource group will be used.'
   resource_group=$(ibmcloud resource groups --output JSON | jq -r '.[] | select (.default == true) .name')
   echo "default resource group: $resource_group"
else
   rg_existing=$(ibmcloud resource group ${resource_group} --output JSON | jq -r '.[] .name')
   if [[ -z $rg_existing ]]; then
      echo "Resource group $resource_group doesn't exist. Try to create it."
      # Create new resource group if not existing, and target it
      ibmcloud resource group-create ${resource_group}
   fi
fi
echo "target resource group: "${resource_group}
ibmcloud target -g ${resource_group}

if [[ -z $version ]]; then
    echo 'IKS version not input use default: 1.23.x'
    export version=$(ibmcloud ks versions | grep 1.23 | sed 's: ::g')
fi


# Create VPC
ibmcloud is vpc-create ${cluster_name}-vpc --resource-group-name ${resource_group}
export vpcID=$(ibmcloud is vpc ${cluster_name}-vpc --output JSON | jq -r .id)
echo "enable ssh login policy"
export security_group=$(ibmcloud is vpc ${vpcID} --output JSON | jq -r .default_security_group.id)
ibmcloud is security-group-rule-add $security_group inbound tcp --port-min 22 --port-max 22 --output JSON

# Create a subnet in each of zones 1 and 2 with 32 addresses
ibmcloud is subnet-create ${cluster_name}-sn1 ${vpcID} --zone ${zone} --ipv4-address-count 32 --resource-group-name ${resource_group}
export subnetID=$(ibmcloud is subnet ${cluster_name}-sn1 --output JSON | jq -r .id)

until ibmcloud ks init --host $iks_endpoint; do sleep 1; done
ibmcloud ks clusters

# Create IKS Cluster, with 1 worker in 1 zone of version "${my_version}".
echo "version:"$version
echo "ibmcloud ks cluster create vpc-gen2 --name ${cluster_name} --vpc-id ${vpcID} --subnet-id ${subnetID} --zone ${zone} --version ${version} --flavor ${flavor} --workers ${workers}"
ibmcloud ks cluster create vpc-gen2 --name ${cluster_name} --vpc-id ${vpcID} --subnet-id ${subnetID} --zone ${zone} --version ${version} --flavor ${flavor} --workers ${workers}


waitClusterReady() {
    echo "Check cluster state."
    CHECK_ATTEMPTS=$1
    MAX_ATTEMPTS_ALLOWED=40
    WAIT_BETWEEN_STATUS_CHECK=60

    cluster_detail=$(ibmcloud ks cluster get --cluster ${cluster_name} --output json)
    cluster_state=$(echo ${cluster_detail} | jq -r '.state')

    if [[ ${cluster_state} != "normal" ]]; then
        echo "The cluster is in state of ${cluster_state}. ${CHECK_ATTEMPTS} attempts of ${MAX_ATTEMPTS_ALLOWED} attempts used."

        if [[ $CHECK_ATTEMPTS -lt $MAX_ATTEMPTS_ALLOWED ]]; then
            echo "Waiting ${WAIT_BETWEEN_STATUS_CHECK} seconds before re-checking cluster status"
            CHECK_ATTEMPTS=$(($CHECK_ATTEMPTS + 1))
            echo "CHECK_ATTEMPTS:"$CHECK_ATTEMPTS
            sleep ${WAIT_BETWEEN_STATUS_CHECK}
            waitClusterReady ${CHECK_ATTEMPTS}
        else
            echo "Cluster is failed to become 'normal' state after $MAX_ATTEMPTS_ALLOWED attempts with $WAIT_BETWEEN_STATUS_CHECK seconds waiting between each check."
            return 99
        fi
    else
        echo "Cluster is ready with state:"$cluster_state
        return 0
    fi

}

waitClusterReady 0

echo "set KUBECONFIG for cluster"
ibmcloud cs cluster config --cluster $cluster_name --admin --network

echo "Creating IKS cluster ended"
