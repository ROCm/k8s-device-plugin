# Installation Guide

This guide walks through the process of installing the AMD GPU device plugin on a Kubernetes cluster.

## Prerequisites

Before installing the AMD GPU device plugin, ensure your environment meets the following requirements:

### System Requirements

- **Kubernetes**: v1.18 or higher
- **AMD GPUs**: ROCm-capable AMD GPU hardware
- **GPU Drivers**: AMD GPU drivers or ROCm stack installed on worker nodes
- **Helm**: v3.2.0 or later (if using the health check feature or GPU Operator)

### Driver Installation

If you haven't installed the AMD GPU drivers yet, follow the official [ROCm Installation Guide](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/tutorial/quick-start.html)

## Installation Steps

Choose one of the following options based on your requirements.

### Option 1: Standard Device Plugin

Use this option if you only need basic GPU allocation without health monitoring.

**Using Pre-defined YAML File**: You can use the pre-defined YAML file provided in this repository. Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp.yaml
```

**Pulling from the Web**: Alternatively, you can directly pull the YAML file from the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

### Option 2: Device Plugin with Health Checks

Use this option if you need GPU health monitoring capabilities in addition to GPU allocation.

#### Step 1: Install AMD Device Metrics Exporter

The health check feature requires the [AMD Device Metrics Exporter](https://instinct.docs.amd.com/projects/device-metrics-exporter/en/latest/index.html) to be installed. This service provides GPU metrics and health information that the device plugin connects to.

Create a `metrics-exporter-values.yaml` file with the following content:

```yaml
platform: k8s
nodeSelector: {} # Optional: Add custom nodeSelector
image:
  repository: docker.io/rocm/device-metrics-exporter
  tag: v1.2.0
  pullPolicy: Always
service:
  type: ClusterIP
  ClusterIP:
    port: 5000
# Enable GRPC socket for device plugin health monitoring
socket:
  enable: true
  path: /var/lib/amd-metrics-exporter/amdgpu_device_metrics_exporter_grpc.socket
  permissions: 0777
volumeMounts:
  - name: socket-dir
    mountPath: /var/lib/amd-metrics-exporter
volumes:
  - name: socket-dir
    hostPath:
      path: /var/lib/amd-metrics-exporter
      type: DirectoryOrCreate
```

Install the metrics exporter with Helm:

```bash
helm install metrics-exporter \
  https://github.com/ROCm/device-metrics-exporter/releases/download/v1.2.0/device-metrics-exporter-charts-v1.2.0.tgz \
  -n kube-system -f metrics-exporter-values.yaml
```

#### Step 2: Install Device Plugin with Health Checks

After successfully installing the metrics exporter, deploy the device plugin with health check capability:

**Using Pre-defined YAML File**: You can use the pre-defined YAML file provided in this repository. Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp-health.yaml
```

**Pulling from the Web**: Alternatively, you can directly pull the YAML file from the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-health.yaml
```

### Option 3: Using AMD GPU Operator

The AMD GPU Operator provides a comprehensive solution that installs and manages:

- AMD GPU device plugin
- Node labeler
- Device metrics exporter
- Driver installation and updates

#### Step 1: Install Cert-Manager

The AMD GPU Operator requires cert-manager for TLS certificate management:

```bash
helm repo add jetstack https://charts.jetstack.io --force-update

helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.15.1 \
  --set crds.enabled=true
```

Verify that cert-manager is installed correctly:

```bash
kubectl get pods -n cert-manager
```

#### Step 2: Install the GPU Operator

Add the AMD Helm repository:

```bash
helm repo add rocm https://rocm.github.io/gpu-operator
helm repo update
```

Install the operator:

```bash
helm install amd-gpu-operator rocm/gpu-operator-charts \
  --namespace kube-amd-gpu \
  --create-namespace
```

#### Step 3: Create a DeviceConfig Custom Resource

After the operator is installed, create a DeviceConfig custom resource to configure the cluster's GPU resources. Create a file named `deviceconfig.yaml`:

```yaml
apiVersion: amd.com/v1alpha1
kind: DeviceConfig
metadata:
  name: cluster-device-config
  namespace: kube-amd-gpu
spec:
  # For using pre-installed drivers
  driver:
    enable: false

  # Configure device plugin
  devicePlugin:
    devicePluginImage: rocm/k8s-device-plugin:latest
    nodeLabellerImage: rocm/k8s-device-plugin:labeller-latest
        
  # Configure metrics exporter
  metricsExporter:
     enable: true
     serviceType: "NodePort"
     nodePort: 32500
     image: docker.io/rocm/device-metrics-exporter:v1.2.0

  # Select nodes with AMD GPUs
  selector:
    feature.node.kubernetes.io/amd-gpu: "true"
```

Deploy the custom resource:

```bash
kubectl apply -f deviceconfig.yaml
```

#### Step 4: Verify the GPU Operator Installation

Check that all operator components are running:

```bash
kubectl get pods -n kube-amd-gpu
```

Verify that nodes with AMD GPUs are properly labeled:

```bash
kubectl get nodes -L feature.node.kubernetes.io/amd-gpu
```

Verify that the AMD GPU resources are available:

```bash
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPU:"status.capacity.amd\.com/gpu"
```

See the [GPU Operator Documentation](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/) for additional information.

### Install Node Labeler (Optional)

The AMD GPU Node Labeler automatically detects and labels nodes with detailed GPU properties, enabling more precise workload scheduling.

The node labeler requires:

- A service account with permissions to modify node labels
- Privileged container access for GPU discovery

Deploy the node labeler using the provided DaemonSet manifest:

```bash
kubectl create -f k8s-ds-amdgpu-labeller.yaml
```

After deployment, nodes with AMD GPUs will be automatically labeled with properties including:

- Device ID
- Product Name
- Driver Version
- VRAM Size
- SIMD Count
- Compute Unit count
- GPU Family information
- Firmware and Feature Versions

The labels are added with two prefixes:

- `amd.com/gpu.*` - Current prefix
- `beta.amd.com/gpu.*` - Legacy prefix (maintained for backwards compatibility)

Verify the labels on your nodes using one of these commands:

```bash
# View all GPU-related labels
kubectl get nodes -o custom-columns=NAME:.metadata.name,LABELS:.metadata.labels

# Filter for current GPU labels
kubectl get nodes --show-labels | grep "amd.com/gpu"

# Filter for legacy GPU labels
kubectl get nodes --show-labels | grep "beta.amd.com/gpu"
```

Example labels for an AMD MI300X GPU:

```text
amd.com/gpu.cu-count=304
amd.com/gpu.device-id=74a1
amd.com/gpu.family=AI
amd.com/gpu.product-name=AMD_Instinct_MI300X_OAM
amd.com/gpu.simd-count=1216
amd.com/gpu.vram=192G
```

### Verify the Device Plugin Installation

Check the status of the pods:

```bash
kubectl get pods -n kube-system
```

Describe the device plugin pod to see logs and events:

```bash
kubectl describe pod <device-plugin-pod-name> -n kube-system
```

After deploying the device plugin, verify that your AMD GPUs are properly recognized as schedulable resources:

```bash
# List all nodes with their AMD GPU capacity
kubectl get nodes -o custom-columns=NAME:.metadata.name,GPU:"status.capacity.amd\.com/gpu"

NAME             GPU
k8s-node-01      8
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

To uninstall the device plugin, delete the DaemonSet using the same manifest file you used for installation:

**If you installed the standard device plugin**:

```bash
kubectl delete -f k8s-ds-amdgpu-dp.yaml
# Or using the web URL
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

**If you installed the device plugin with health checks**:

```bash
kubectl delete -f k8s-ds-amdgpu-dp-health.yaml
# Or using the web URL
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-health.yaml
```

### Uninstalling the GPU Operator

If you installed using the AMD GPU Operator (Option 3), uninstall it with:

```bash
# First delete the device config custom resource
kubectl delete deviceconfig -n kube-amd-gpu --all

# Wait for KMM modules to be unloaded
kubectl get modules -n kube-amd-gpu

# Uninstall the operator
helm uninstall amd-gpu-operator -n kube-amd-gpu

# Optionally uninstall cert-manager if no longer needed
helm uninstall cert-manager -n cert-manager
```
