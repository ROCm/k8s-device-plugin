# AMD GPU Kubernetes Node Labeller

## Introduction

This tool automatically label nodes with GPU properties if a node has one or more AMD GPU installed.  This tool leverage [controller-runtime][cr] in the spirit of [Custom Resource Definition (CRD)][crd] controller even though we do not define a Custom Resource.

## Prerequisites

* Node Labeller needs to be run inside a Kubernetes Pod
* The node's hostname need's to be made available inside the container in a text file with the path `/labeller/hostname`.
* The Pod containing the Labeller needs to be deployed by a service account with sufficient API access.  This can be achieved through the use of [ClusterRole][rcr] and [ClusterRoleBinding][rcrb].
  * apiGroups: core ("")
  * resources: `nodes`
  * verbs: `watch`, `get`, `list`, `update`

## Deployment

The Labeller needs to be run on all the nodes that are equipped with AMD GPU.  The simplist way of doing so is to create a Kubernete [DaemonSet][ds], which runs a copy of a pod on all (or some) Nodes in the cluster.  An example configuration is available [here](../../k8s-ds-amdgpu-labeller.yaml). This labeller required privileged container for gpu feature discovery. It is recommended to consult with your cluster administrator or security expert to ensure appropriate security measures are in place.

The Labeller currently creates node label for the following AMD GPU properties:

* Device ID (-device-id)
* Product Name (-product-name)
* Driver Version (-driver-version)
* Driver Source Version (-driveri-src-version)
* VRAM Size (-vram)
* Number of SIMD (-simd-count)
* Number of Compute Unit (-cu-count)
* Firmware and Feature Versions (-firmware)
* GPU Family, in two letters acronym (-family)
  * SI - Southern Islands
  * CI - Sea Islands
  * KV - Kaveri
  * VI - Volcanic Islands
  * CZ - Carrizo
  * AI - Arctic Islands
  * RV - Raven
  * NV - Navi
  * VGH - Van Gogh
  * GC\_11\_0\_0 - GC 11.0.0
  * YC - Yellow Carp
  * GC\_11\_0\_1 - GC 11.0.1
  * GC\_10\_3\_6 - GC 10.3.6
  * GC\_10\_3\_7 - GC 10.3.7
  * GC\_11\_5\_0 - GC 11.5.0

Example result

    $ kubectl describe node cluster-node-23
    Name:               cluster-node-23
    Roles:              <none>
    Labels:             beta.amd.com/gpu.cu-count.64=1
                        beta.amd.com/gpu.device-id.6860=1
                        beta.amd.com/gpu.family.AI=1
                        beta.amd.com/gpu.simd-count.256=1
                        beta.amd.com/gpu.vram.16G=1
                        beta.kubernetes.io/arch=amd64
                        beta.kubernetes.io/os=linux
                        kubernetes.io/hostname=cluster-node-23
    Annotations:        kubeadm.alpha.kubernetes.io/cri-socket: /var/run/dockershim.sock
                        node.alpha.kubernetes.io/ttl: 0
    ......

You can selectively expose the GPU properties by passing in the corresponding flag.  For example, to only explose VRAM and Device ID as node labels, run the Node Labeller like this:

    $ ./k8s-node-labeller -vram -device-id

## Usage example with label selector

Once the Node Labeller is deployed and functional, you can select specific nodes via Kubernetes' [label selector][ls].  For example, to select nodes with only 8GB of VRAM:

    $ kubectl get nodes -l beta.amd.com/gpu.vram.8G

## Limitations

While container scheduling via label selector works for heterogeneous cluster, it requires homogenous nodes.  For example, you can have a node with just Fiji and just Vega10 in the same cluster but not a node that has both Fiji and Vg10 cards in it.

## Notes

## TODOs

[ls]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
[ds]: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
[crd]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
[cr]: https://github.com/kubernetes-sigs/controller-runtime
[rcr]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#role-and-clusterrole
[rcrb]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#rolebinding-and-clusterrolebinding
