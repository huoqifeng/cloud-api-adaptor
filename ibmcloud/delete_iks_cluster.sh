#!/bin/bash

# requires ibmcloud cli, cloud-object-storage, infrastructure-service, container-service plugin and Kubernetes command line tool
# following https://cloud.ibm.com/docs/cli?topic=cli-install-ibmcloud-cli
#           https://cloud.ibm.com/docs/cli?topic=cli-install-devtools-manually
# example:
#   curl -fsSL https://clis.cloud.ibm.com/install/linux | sh
#   ibmcloud plugin install container-service -f
#   ibmcloud plugin install infrastructure-service -f
#   ibmcloud plugin install cloud-object-storage -f
#   curl --progress-bar -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl


set -o errexit

printHelp() {
    cat << CMD
    Usage: $0 <cmd> [<args>]

    args:

      -name                The name of the cluster which you want to delete.
      -region              The region of cluster created. Such as 'us-south'.
      -apikey              API Keys for your account in IBM Cloud.

    Sample Usage:

      ./delete_iks_cluster.sh name="testcluster" apikey=${myAPIKey} region="us-south"

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
    echo 'Please set name arg to the name of the Cluster you want to clean.'
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


echo "production environment"
export endpoint="cloud.ibm.com"
export iks_endpoint="https://containers.cloud.ibm.com"


waitClusterDeleted() {
    CHECK_ATTEMPTS=$1
    MAX_ATTEMPTS_ALLOWED=30
    WAIT_BETWEEN_STATUS_CHECK=30

    ibmcloud cs cluster get --cluster $cluster_name &> /dev/null
    if [[ $? -ne 0 ]]; then
        echo "cluster $cluster_name is deleted"
        return 0
    else
        echo "${CHECK_ATTEMPTS} attempts of ${MAX_ATTEMPTS_ALLOWED} attempts used."
        if [[ $CHECK_ATTEMPTS -lt $MAX_ATTEMPTS_ALLOWED ]]; then
            echo "Waiting ${WAIT_BETWEEN_STATUS_CHECK} seconds before re-checking cluster status"
            CHECK_ATTEMPTS=$(($CHECK_ATTEMPTS + 1))
            sleep ${WAIT_BETWEEN_STATUS_CHECK}
            waitClusterDeleted ${CHECK_ATTEMPTS}
        else
            echo "cluster $cluster_name is still not deleted after $MAX_ATTEMPTS_ALLOWED attempts with $WAIT_BETWEEN_STATUS_CHECK seconds waiting between each check."
            return 99
        fi
    fi
}

deleteSubnet() {
    CHECK_ATTEMPTS=$1
    MAX_ATTEMPTS_ALLOWED=20
    WAIT_BETWEEN_STATUS_CHECK=10

    delete_subnet_output=$(ibmcloud is subnet-delete $subnet -f -q --output JSON)
    result=$(echo $delete_subnet_output | jq '.[].result')
    echo $result
    if [[ $result == "true" ]]; then
        echo "subnet $subnet is deleted"
        return 0
    else
        grep_subnet_in_use=$(echo $delete_subnet_output | grep subnet_in_use_network_interface_exists)
        if [[ -n $grep_subnet_in_use ]]; then
            echo "subnet $subnet is still in use"
            if [[ $CHECK_ATTEMPTS -lt $MAX_ATTEMPTS_ALLOWED ]]; then
                echo "Waiting ${WAIT_BETWEEN_STATUS_CHECK} seconds before deleting subnet again"
                CHECK_ATTEMPTS=$(($CHECK_ATTEMPTS + 1))
                sleep ${WAIT_BETWEEN_STATUS_CHECK}
                deleteSubnet ${CHECK_ATTEMPTS}
            else
                echo "subnet $subnet is still in use after $MAX_ATTEMPTS_ALLOWED attempts with $WAIT_BETWEEN_STATUS_CHECK seconds waiting between each attempt."
                return 99
            fi
        else
            echo "failed to delete subnet $subnet"
            return 99
        fi
    fi
}

deleteVPC() {
    CHECK_ATTEMPTS=$1
    MAX_ATTEMPTS_ALLOWED=6
    WAIT_BETWEEN_STATUS_CHECK=10

    delete_vpc_output=$(ibmcloud is vpc-delete $vpc -f -q --output JSON)
    result=$(echo $delete_vpc_output | jq '.[].result')
    echo $result
    if [[ $result == "true" ]]; then
        echo "vpc $vpc is deleted"
        return 0
    else
        grep_vpc_in_use=$(echo $delete_vpc_output | grep vpc_in_use)
        if [[ -n $grep_vpc_in_use ]]; then
            echo "vpc $vpc is still in use"
            if [[ $CHECK_ATTEMPTS -lt $MAX_ATTEMPTS_ALLOWED ]]; then
                echo "Waiting ${WAIT_BETWEEN_STATUS_CHECK} seconds before deleting vpc again"
                CHECK_ATTEMPTS=$(($CHECK_ATTEMPTS + 1))
                sleep ${WAIT_BETWEEN_STATUS_CHECK}
                deleteVPC ${CHECK_ATTEMPTS}
            else
                echo "vpc $vpc is still in use after $MAX_ATTEMPTS_ALLOWED attempts with $WAIT_BETWEEN_STATUS_CHECK seconds waiting between each attempt."
                return 99
            fi
        else
            echo "failed to delete vpc $vpc"
            return 99
        fi
    fi
}

# Login to test.cloud, use us-south and a named resource group
ibmcloud login -a ${endpoint} -r ${region} --apikey ${apikey}

if [[ -n $cluster_name ]]; then
    set +e

    echo "Delete cluster and resources via cluster name: $cluster_name"
    ibmcloud cs cluster config --cluster $cluster_name --admin --network

    not_found_array=()
    failed_array=()
    succeed_array=()

    ibmcloud cs cluster get --cluster $cluster_name -q &> /dev/null
    if [[ $? -ne 0 ]]; then
        not_found_array=("${not_found_array[@]}" "cluster: $cluster_name")
    else
        echo "Delete cluster: $cluster_name ..."
        ibmcloud cs cluster rm --cluster $cluster_name -f --force-delete-storage -q
        waitClusterDeleted 1
        if [[ $? -eq 0 ]]; then
            succeed_array=("${succeed_array[@]}" "cluster: $cluster_name")
        else
            failed_array=("${failed_array[@]}" "cluster: $cluster_name")
        fi
    fi
    
    # try to remove subnet and vpc with name pattern.
    if [[ -z $subnet ]]; then
        subnet="${cluster_name}-sn1"
    fi
    if [[ -z $vpc ]]; then
        vpc="${cluster_name}-vpc"
    fi

    ibmcloud is subnet $subnet -q &> /dev/null
    if [[ $? -ne 0 ]]; then
        not_found_array=("${not_found_array[@]}" "subnet: $subnet")
    else
        echo "Delete subnet: $subnet ..."
        deleteSubnet 1
        if [[ $? -eq 0 ]]; then
            succeed_array=("${succeed_array[@]}" "subnet: $subnet")
        else
            failed_array=("${failed_array[@]}" "subnet: $subnet")
        fi
    fi

    ibmcloud is vpc $vpc -q &> /dev/null
    if [[ $? -ne 0 ]]; then
        not_found_array=("${not_found_array[@]}" "vpc: $vpc")
    else
        echo "Delete VPC: $vpc ..."
        deleteVPC 1
        if [[ $? -eq 0 ]]; then
            succeed_array=("${succeed_array[@]}" "vpc: $vpc")
        else
            failed_array=("${failed_array[@]}" "vpc: $vpc")
        fi
    fi
    
    # Print summary
    echo "----------------- SUMMARY -----------------"
    if [[ ${#succeed_array[@]} -gt 0 ]]; then
        echo " "
        echo "* Succeed to delete:"
        for element in "${succeed_array[@]}"; do
            echo $element
        done
    fi

    if [[ ${#not_found_array[@]} -gt 0 ]]; then
        echo " "
        echo "* Not found:"
        for element in "${not_found_array[@]}"; do
            echo $element
        done
    fi

    if [[ ${#failed_array[@]} -gt 0 ]]; then
        echo " "
        echo "* Failed to delete:"
        for element in "${failed_array[@]}"; do
            echo $element
        done
    fi

    set -e
fi