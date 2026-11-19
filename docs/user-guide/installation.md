# Installation guide

This guide describes how to install the AMD GPU device plugin on a Kubernetes cluster.

## Prerequisites

Before installing the AMD GPU device plugin, ensure your environment meets the following requirements:

### System requirements

- **Kubernetes**: v1.18 or higher
- **AMD GPUs**: ROCm-capable AMD GPU hardware
- **GPU drivers**: AMD GPU drivers or ROCm stack installed on worker nodes
- **Helm**: v3.2.0 or later (if using the health check feature or GPU Operator)

### Driver installation

If you haven't installed the AMD GPU drivers yet, follow the [ROCm installation guide](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/tutorial/quick-start.html).

## Installation steps

Choose one of the following options based on your requirements.

### Option 1: Standard device plugin

Use this option if you only need basic GPU allocation without health monitoring.

**Using the predefined YAML file**: Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp.yaml
```

**Pulling from the web**: Create the DaemonSet from the YAML file in the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

### Option 1.a: Standard device plugin with init container

Use this option when the `amdgpu` driver might not be loaded before the device plugin starts. This deployment has an init container that waits for the `amdgpu` driver to load before it starts the main plugin container.

**Using the predefined YAML file**: Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp-with-init-container.yaml
```

**Pulling from the web**: Create the DaemonSet from the YAML file in the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-with-init-container.yaml
```

### Option 2: Device plugin with health checks

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
# Enable gRPC socket for device plugin health monitoring
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

#### Step 2: Install the device plugin with health checks

After successfully installing the metrics exporter, deploy the device plugin with health check capability:

**Using the predefined YAML file**: Run the following command:

```bash
kubectl create -f k8s-ds-amdgpu-dp-health.yaml
```

**Pulling from the web**: Create the DaemonSet from the YAML file in the repository:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-health.yaml
```

### Option 3: Using AMD GPU Operator

The AMD GPU Operator provides a comprehensive solution that installs and manages:

- AMD GPU device plugin
- Node labeler
- Device metrics exporter
- Driver installation and updates

See the [AMD GPU Operator documentation](https://instinct.docs.amd.com/projects/gpu-operator/en/latest/) for installation instructions and additional information.

### Install the node labeler (optional)

The AMD GPU Node Labeler automatically detects and labels nodes with detailed GPU properties, enabling more precise workload scheduling.

The node labeler requires:

- A service account with permissions to modify node labels
- Privileged container access for GPU discovery

Deploy the node labeler using the provided DaemonSet manifest:

```bash
kubectl create -f k8s-ds-amdgpu-labeller.yaml
```

With the provided manifest, the node labeler adds these labels to nodes with AMD GPUs:

- Device ID
- Product name
- VRAM size
- SIMD count
- Compute unit count
- GPU family

To add other labels, such as the driver version, firmware versions, or partition information, add the corresponding flags (for example, `-driver-version` or `-firmware`) to the `args` in `k8s-ds-amdgpu-labeller.yaml`.

The labels are added with two prefixes:

- `amd.com/gpu.*` - Current prefix
- `beta.amd.com/gpu.*` - Legacy prefix (maintained for backward compatibility)

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

### Verify the device plugin installation

Check the status of the pods:

```bash
kubectl get pods -n kube-system
```

Describe the device plugin pod to see its status and events:

```bash
kubectl describe pod <device-plugin-pod-name> -n kube-system
```

After deploying the device plugin, verify that Kubernetes recognizes your AMD GPUs as schedulable resources:

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

## Uninstalling the device plugin

To uninstall the device plugin, delete the DaemonSet using the same manifest file you used for installation:

If you installed the standard device plugin (Option 1):

```bash
kubectl delete -f k8s-ds-amdgpu-dp.yaml
# Or using the web URL
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
```

If you installed the device plugin with an init container (Option 1.a):

```bash
kubectl delete -f k8s-ds-amdgpu-dp-with-init-container.yaml
# Or using the web URL
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-with-init-container.yaml
```

If you installed the device plugin with health checks (Option 2):

```bash
kubectl delete -f k8s-ds-amdgpu-dp-health.yaml
# Or using the web URL
kubectl delete -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-health.yaml
```

If you installed the node labeler:

```bash
kubectl delete -f k8s-ds-amdgpu-labeller.yaml
```
