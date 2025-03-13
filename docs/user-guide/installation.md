# Installation Guide

This guide walks through the process of installing the AMD GPU device plugin on a Kubernetes cluster.

## Prerequisites

Before installing the AMD GPU device plugin, ensure your environment meets the following requirements:

### System Requirements

- **Kubernetes**: v1.18 or higher
- **AMD GPUs**: ROCm-capable AMD GPU hardware
- **GPU Drivers**: AMD GPU drivers or ROCm stack installed on worker nodes
- **kubectl**: Configured with access to your cluster

### Driver Installation

If you haven't installed the AMD GPU drivers yet, follow the official guides:

- [ROCm Installation Guide](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/tutorial/quick-start.html)
- [AMD GPU Driver Installation Guide](https://amdgpu-install.readthedocs.io/en/latest/)

## Installation Steps

### Step 1: Create a DaemonSet

The simplest way to deploy the device plugin is by creating a Kubernetes DaemonSet. This will ensure that the plugin runs on all nodes with AMD GPUs.

**Using Pre-defined YAML File**: You can use the pre-defined YAML file provided in this repository. Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp.yaml
```

**Pulling from the Web**: Alternatively, you can directly pull the YAML file from the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

### Step 2: Enable Device Health Checks (Optional)

If you want to enable the experimental device health check feature, you need to set the `--allow-privileged=true` flag for the kube-apiserver. After that, use the following command to deploy the health check DaemonSet:

```bash
kubectl create -f k8s-ds-amdgpu-dp-health.yaml
```

### Step 3: Verify the Installation

After deploying the DaemonSet, verify that the device plugin is running correctly:

Check the status of the pods:

```bash
kubectl get pods -n kube-system
```

Describe the device plugin pod to see logs and events:

```bash
kubectl describe pod <device-plugin-pod-name> -n kube-system
```

## Troubleshooting

If the device plugin pods are not running, check the logs:

```bash
kubectl logs -n kube-system <amdgpu-device-plugin-pod-name>
```

Common issues include:

- GPU drivers not installed correctly
- ROCm stack not installed or misconfigured
- Insufficient permissions for the device plugin to access GPU devices

## Uninstalling the Device Plugin

To uninstall the device plugin, delete the DaemonSet:

```bash
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```
