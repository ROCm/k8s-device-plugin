# AMD GPU Device Plugin for Kubernetes

[![Go Report Card](https://goreportcard.com/badge/github.com/ROCm/k8s-device-plugin)](https://goreportcard.com/report/github.com/ROCm/k8s-device-plugin)

## Introduction

This is a [Kubernetes][k8s] [device plugin][dp] implementation that enables the registration of AMD GPU in a container cluster for compute workload.  With the appropriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD GPU.

This plugin is required by tools such as the [AMD GPU Operator](https://github.com/ROCm/gpu-operator) to expose AMD GPUs as schedulable resources.

More information about [ROCm][rocm].

## Prerequisites

* [ROCm capable machines][sysreq]
* [kubeadm capable machines][kubeadm] (if you are using kubeadm to deploy your k8s cluster)
* [ROCm kernel][rock] ([Installation guide][rocminstall]) or latest AMD GPU Linux driver ([Installation guide][amdgpuinstall])
* A [Kubernetes deployment][k8sinstall]
* If device health checks are enabled, the pods must be allowed to run in privileged mode (for example the `--allow-privileged=true` flag for kube-apiserver), in order to access `/dev/kfd`

## Limitations

* This plugin targets Kubernetes v1.18+.

## Deployment

The device plugin needs to be run on all the nodes that are equipped with AMD GPU.  The simplest way of doing so is to create a Kubernetes [DaemonSet][ds], which runs a copy of a pod on all (or some) Nodes in the cluster.  We have a pre-built Docker image on [DockerHub][dhk8samdgpudp] that you can use for your DaemonSet.  This repository also has a pre-defined yaml file named `k8s-ds-amdgpu-dp.yaml`.  You can create a DaemonSet in your Kubernetes cluster by running this command:

```bash
kubectl create -f k8s-ds-amdgpu-dp.yaml
```

or directly pull from the web using

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

If you want to enable the experimental device health check, please use `k8s-ds-amdgpu-dp-health.yaml` **after** `--allow-privileged=true` is set for kube-apiserver.

### Helm Chart

If you want to deploy this device plugin using Helm, a [Helm Chart][helmamdgpu] is available via [Artifact Hub][artifacthub].

## Example workload

You can restrict workloads to a node with a GPU by adding `resources.limits` to the pod definition.  An example pod definition is provided in `example/pod/alexnet-gpu.yaml`.  This pod runs the timing benchmark for AlexNet on AMD GPU and then goes to sleep. You can create the pod by running:

```bash
kubectl create -f alexnet-gpu.yaml
```

or
bash
```
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/alexnet-gpu.yaml
```

and then check the pod status by running

```bash
kubectl describe pods
```

After the pod is created and running, you can see the benchmark result by running:

```bash
kubectl logs alexnet-tf-gpu-pod alexnet-tf-gpu-container
```

For comparison, an example pod definition of running the same benchmark with CPU is provided in `example/pod/alexnet-cpu.yaml`.

## Labelling node with additional GPU properties

Please see [AMD GPU Kubernetes Node Labeller](cmd/k8s-node-labeller/README.md) for details.  An example configuration is in [k8s-ds-amdgpu-labeller.yaml](k8s-ds-amdgpu-labeller.yaml):

```bash
kubectl create -f k8s-ds-amdgpu-labeller.yaml
```

or

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-labeller.yaml
```

# Health per GPU

* Extends more granular health detection per GPU using the exporter health
  service over grpc socket service mounted on /var/lib/amd-metrics-exporter/

# GPU Time-Slicing (Virtual Devices)

GPU time-slicing allows a single physical AMD GPU to be advertised as multiple virtual devices to Kubernetes, enabling multiple pods to share a GPU via OS-level scheduling. This is a Kubernetes-level overcommit — all virtual slices of the same physical GPU share the same `/dev/kfd` and `/dev/dri/renderD*` devices, so pods compete for VRAM and compute at runtime.

| Flag / Field | Type | Default | Valid Range | Description |
|--------------|------|---------|-------------|-------------|
| `--replicas` | int | `1` | `≥ 1` | Number of virtual device slices per physical GPU |

Setting `--replicas=1` (or omitting it) produces behavior identical to the upstream plugin.

## Quick Start

Add the `--replicas` flag to the DaemonSet container args:

```yaml
containers:
- image: rocm/k8s-device-plugin
  name: amdgpu-dp-cntr
  args:
    - "./k8s-device-plugin"
    - "--replicas=4"
```

That's it. A node with 2 physical GPUs will now report `8` under `amd.com/gpu`.

## Verification

```bash
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPU:"status.capacity.amd\.com/gpu"
```

Two pods each requesting `amd.com/gpu: 1` can be scheduled on a node with a single physical GPU when `replicas >= 2`.

## Caveats

- **No hardware isolation**: All virtual slices share the same physical GPU. Pods compete for VRAM and compute resources at the OS scheduler level.
- **No MIG equivalent**: Unlike NVIDIA MIG, there is no hardware-level partitioning. Time-slicing provides Kubernetes scheduling flexibility but no performance guarantees.


* This plugin uses [`go modules`][gm] for dependencies management
* Please consult the `Dockerfile` on how to build and use this plugin independent of a docker image

## TODOs

* ~~Add proper GPU health check (health check without `/dev/kfd` access.)~~

[artifacthub]: https://artifacthub.io/packages/helm/amd-gpu-helm/amd-gpu
[ds]: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
[dp]: https://kubernetes.io/docs/concepts/cluster-administration/device-plugins/
[helmamdgpu]: https://artifacthub.io/packages/helm/amd-gpu-helm/amd-gpu
[rocm]: https://rocm.docs.amd.com/en/latest/what-is-rocm.html
[rock]: https://github.com/ROCm/ROCK-Kernel-Driver
[rocminstall]: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/tutorial/quick-start.html
[amdgpuinstall]: https://amdgpu-install.readthedocs.io/en/latest/
[sysreq]: https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/system-requirements.html
[gm]: https://blog.golang.org/using-go-modules
[kubeadm]: https://kubernetes.io/docs/setup/independent/install-kubeadm/#before-you-begin
[k8sinstall]: https://kubernetes.io/docs/setup/independent/install-kubeadm
[k8s]: https://kubernetes.io
[dhk8samdgpudp]: https://hub.docker.com/r/rocm/k8s-device-plugin/
