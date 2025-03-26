# AMD GPU Device Plugin for Kubernetes

The AMD GPU Device Plugin for Kubernetes enables the use of AMD GPUs as schedulable resources in Kubernetes clusters. This plugin allows you to run GPU-accelerated workloads such as machine learning, scientific computing, and visualization applications on Kubernetes.

## Features

- Implements the Kubernetes Device Plugin API for AMD GPUs
- Exposes AMD GPUs as `amd.com/gpu` resources in Kubernetes
- Provides automated node labeling with detailed GPU properties (device ID, VRAM, compute units, etc.)
- Enables fine-grained GPU allocation for containers

## System Requirements

- **Kubernetes**: v1.18 or higher
- **AMD GPUs**: ROCm-capable AMD GPU hardware
- **GPU Drivers**: AMD GPU drivers or ROCm stack installed on worker nodes

See the [ROCm System Requirements](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/reference/system-requirements.html) for detailed hardware compatibility information.

## Quick Start

To deploy the device plugin, run it on all nodes equipped with AMD GPUs. The simplest way to do this is by creating a Kubernetes [DaemonSet](https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/). A pre-built Docker image is available on [DockerHub](https://hub.docker.com/r/rocm/k8s-device-plugin), and a predefined YAML file named [k8s-ds-amdgpu-dp.yaml](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml) is included in this repository.

Create a DaemonSet in your Kubernetes cluster with the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp.yaml
```

Alternatively, you can pull directly from the web:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

### Deploy the Node Labeler (Optional)

For enhanced GPU discovery and scheduling, deploy the AMD GPU Node Labeler:

```bash
kubectl create -f k8s-ds-amdgpu-labeller.yaml
```

This will automatically label nodes with GPU-specific information such as VRAM size, compute units, and device IDs.

### Verify Installation

After deploying the device plugin, verify that your AMD GPUs are properly recognized as schedulable resources:

```bash
# List all nodes with their AMD GPU capacity
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPU:"status.capacity.amd\.com/gpu"

NAME             GPU
k8s-node-01      8
```

## Example Workload

You can restrict workloads to a node with a GPU by adding `resources.limits` to the pod definition. An example pod definition is provided in [example/pod/pytorch.yaml](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/pytorch.yaml). Create the pod by running:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/pytorch.yaml
```

Check the pod status with:

```bash
kubectl describe pods
```

After the pod is running, view the benchmark results with:

```bash
kubectl pytorch-gpu-pod-example
```

## Contributing

We welcome contributions to this project! Please refer to the [Development Guidelines](contributing/development.md) for details on how to get involved.
