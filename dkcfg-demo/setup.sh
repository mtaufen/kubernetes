#!/bin/bash

##############################################################
## Sets up a straw-man cluster to demo a "feature rollout".
## This is a bit of a hack, because we roll out a feature that
## was gated both at the API server and Kubelet level.
## Due to the way the turnup scripts are written, we have
## to start the whole cluster with the feature gate enabled, and then
## disable it on the nodes before "rolling it out" to simulate
## having a feature enabled on the master but not on nodes.
##############################################################


set -e

# aliases
DEMO="$GOPATH/src/k8s.io/kubernetes/dkcfg-demo"
KUBECTL="$GOPATH/src/k8s.io/kubernetes/cluster/kubectl.sh"

# configure the cluster
export KUBE_NODE_OS_DISTRIBUTION="cos"
export KUBE_FEATURE_GATES="DynamicKubeletConfig=true,AppArmor=true"
export KUBELET_TEST_ARGS="--dynamic-config-dir=/var/run/kubelet/dynamic-config"

# spin up the cluster, note this automatically sets up kubeconfig file too
$GOPATH/src/k8s.io/kubernetes/cluster/kube-up.sh

# start kubectl proxy in the background
$KUBECTL proxy --port=8001 &

# get all the node names
allnodes=$(kubectl get nodes -o name)
if [[ $? -ne 0 ]]; then
	echo "unexpected error getting nodes"
	exit 1
fi

# master node's name
master=$(echo $allnodes | head -n 1 | sed -e 's/nodes\///g')

# ignore the master, which shows up first in the list (fragile, but works for demo)
let "numnodes = $(echo $allnodes | wc -l) - 1"

# note the first thing in the array is at index 1, we hackishly handle this later
# we also remove the leading "nodes/" from kubectl get no -o name here
nodes=($(echo $allnodes | tail -n $numnodes | sed -e 's/nodes\///g'))

# get a node's name
nodename=${nodes[1]}

# curl kubelet config from configz into a file called `kubelet`
curl http://localhost:8001/api/v1/proxy/nodes/$nodename/configz | jq .kubeletconfig > $DEMO/kubeletconfig

# generate a kubelet configuration that turns AppArmor off
cat $DEMO/kubeletconfig | jq '.featureGates|="DynamicKubeletConfig=true,AppArmor=false"' > $DEMO/kubelet

# create the ConfigMap for the AppArmor-off configuration on the API server
# TODO: add --append-hash part of workflow to this
out=$($KUBECTL create configmap apparmor-off --from-file=$DEMO/kubelet -o json)
name=$(echo $out | jq --raw-output '.metadata.name')
namespace=$(echo $out | jq --raw-output '.metadata.namespace')
uid=$(echo $out | jq --raw-output '.metadata.uid')
echo "created configmap $namespace/$name with uid $uid"

patch="{\"spec\":{\"configSource\":{\"configMapRef\":{\"name\":\"$name\",\"namespace\":\"$namespace\",\"uid\":\"$uid\"}}}}"

# roll this configuration out to the nodes
for node in $nodes
do
	if [[ ! -z $node ]]; then
		echo "turning AppArmor off for node: $node"
		$KUBECTL patch node $node -p $patch
	fi
done

# check that the ConfigOK status indicates the new config is in use by each node
expectConfigOk=""
while true;
do
	let "num = 0"
	for node in $nodes
	do
		if [[ ! -z $node ]]; then
			configOk=$($KUBECTL get nodes $node -o json | jq --raw-output '.status.conditions|map(select(.type=="ConfigOK"))|.[].message')
			# if it's using the desired config, keep counting
			if [[ $configOk == $expectConfigOk ]]; then
				let "num += 1"
				echo "$node using expected config"
			fi
			echo "$node not using expected config"
		fi
	done
	# were all the nodes as expected?
	if [[ $num -eq $numnodes ]]; then
		echo "all nodes using expected config"
		break
	fi
	# retry
	sleep=1
	echo "$num/$numnodes nodes have accepted the new config, will sleep $sleep second(s) and then re-check"
	sleep $sleep;
done

# create pods that need AppArmor, one targeted to each node
for node in $nodes
do
	if [[ ! -z $node ]]; then
		echo "creating pod with node selector for node: $node"
		cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
    name: my-${node}-pod
    annotations:
        container.apparmor.security.beta.kubernetes.io/app-armor: runtime/default
spec:
    nodeSelector:
        kubernetes.io/hostname: ${node}
    containers:
      - name: app-armor
        image: busybox
        command: [ "sh", "-c", "sleep 1h" ]
EOF
	fi
done