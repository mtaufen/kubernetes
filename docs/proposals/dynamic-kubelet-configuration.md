<!-- BEGIN MUNGE: UNVERSIONED_WARNING -->

<!-- BEGIN STRIP_FOR_RELEASE -->

<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">

<h2>PLEASE NOTE: This document applies to the HEAD of the source tree</h2>

If you are using a released version of Kubernetes, you should
refer to the docs that go with that version.

Documentation for other releases can be found at
[releases.k8s.io](http://releases.k8s.io).
</strong>
--

<!-- END STRIP_FOR_RELEASE -->

<!-- END MUNGE: UNVERSIONED_WARNING -->

# Dynamic Kubelet Configuration

## Abstract

A proposal for making it possible to (re)configure Kubelets in a live cluster by providing config via the API server. Some subordinate items include local checkpointing of Kubelet configuration and the ability for the Kubelet to read config from a file on disk, rather than from command line flags.

## Motivation

The Kubelet is currently configured via command-line flags. This is painful for a number of reasons:
- It makes it difficult to change the way Kubelets are configured in a running cluster, because it is often tedious to change the Kubelet startup configuration (without adding your own configuration management system e.g. Ansible, Salt, Puppet).
- It makes it difficult to manage different Kubelet configurations for different nodes, e.g. if you want to canary a new config or slowly flip the switch on a new feature.
- The current lack of versioned Kubelet configuration means that any time we change Kubelet flags, we risk breaking someone's setup.

## Example Use Cases

- Staged rollout of configuration chages, including tuning adjustments and new Kubelet features.
- More easily run tests with different Kubelet configurations (can dynamicly set the configuration from the test, via the API server).

## Primary Goals of the Design

K8s should:

- Provide a versioned object to represent the Kubelet configuration.
- Provide the ability to specify a dynamic configuration source to each Kubelet (for example, provide the name of a `ConfigMap` that contains the configuration).
- Provide a way to share the same configuration source between nodes.
- Protect against bad configuration pushes.
- Recommend, but not mandate, the basics of a workflow for updating configuration.

## Additional Goals

- Add Kubelet support for consuming configuration via a file on disk. This aids work towards deprecating flags in favor of on-disk configuration. This functionality can also be reused for locak checkpointing of Kubelet configuration.
- Make it possible to opt-out of remote configuration as an extra layer of protection. This should probably be a flag so that you can't dynamically turn off dynamic config by accident.

## Design

Two really important questions:
1. How should one organize and represent configuration in a cluster?
2. How should one orchestrate changes to that configuration?



### Definitions*||||||||||||||||||||||||||||||||||||*


### Organization of the Kubelet's Configuration Type

- as part of the goal of getting off flags, we should have a separate flags struct e.g. #32215
- experimental fields

### Representation and Organization of Kubelet Configuration in a Cluster

- As a cluster-level object, Kubelet's configuration should be stored in a `ConfigMap` object.
- On local disk, the Kubelet's configuration should be stored in a `.json` or `.yaml` file.
- The Kubelet's configuration should be, at least initially, organized in the cluster as a structured monolith. 
    + *Structured*, so that it is readable.
    + *Monolithic*, to provide atomicity over the entire configuration object.
        * If leaky boundaries occur between the substructures, we don't want the problem of coordinating non-atomic updates across separately-referenced substructures.
        * If, in the future, the ability to independently orchestrate the substructures of the configuration is desired, we can move down that road. But today this is probably overkill, because most cluster configuration is eventually homogeneous. Even in a non-homogeneously configured cluster, we would have to carefully consider the downside of losing atomicity on the config object against the upside of more flexibility for splitting up configuration responsibility.

### Referencing Configuration

e.g. how do you tell the kubelet what to use?
- cluster-level - give the configmap name or use an object ref
- on-disk - specify a path


    This likely means that the Kubelet configuration will be atomic in the form of a string blob (JSON or YAML) stored in the value associated with one of a `ConfigMap`'s keys.

### Orchestration of configuration

#### Robust Kubelet behavior

#### Recommendations regarding update workflow


## Additional concerns not-yet-addressed

- A way to query/monitor the config in-use on a given node. (today this is configz)
- RBAC on ConfigMaps
- 






<!-- BEGIN MUNGE: GENERATED_ANALYTICS -->
[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/proposals/dynamic-kubelet-settings.md?pixel)]()
<!-- END MUNGE: GENERATED_ANALYTICS -->
