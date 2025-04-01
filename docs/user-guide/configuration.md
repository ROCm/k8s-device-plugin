# Configuration Options

This document outlines the configuration options available for the AMD GPU device plugin for Kubernetes.

## Environment Variables

The device plugin can be configured using the following environment variables:

| Environment Variable | Type | Default | Description |
|-----|------|---------|-------------|
| `AMD_GPU_DEVICE_COUNT` | Integer | Auto-detected | Number of AMD GPUs available on the node |

### Why Limit GPU Exposure?

There are several reasons an administrator might want to limit the number of GPUs exposed to Kubernetes:

1. **Resource Partitioning**: Reserve some GPUs for non-Kubernetes workloads running on the same node
2. **Testing and Development**: Test applications with restricted GPU access before deploying to production
3. **Mixed Workload Management**: Allocate specific GPUs to different teams or applications based on priority
4. **High Availability**: Keep backup GPUs available for failover scenarios

Setting `AMD_GPU_DEVICE_COUNT` to a value lower than the physical count ensures only a subset of GPUs are made available as Kubernetes resources.

## Command-Line Flags

The device plugin supports the following command-line flags:

| Flag | Default | Description |
|-----|------|-------------|
| `--kubelet-url` | `http://localhost:10250` | The URL of the kubelet for device plugin registration |
| `--pulse` | `0` | Time between health check polling in seconds. Set to 0 to disable. |

## Configuration File

You can also provide a configuration file in YAML format to customize the plugin's behavior:

```yaml
gpu:
  device_count: 2
```

### Using the Configuration File

To use the configuration file:

1. Create a YAML file with your desired settings (like the example above)
2. Mount this file into the device plugin container

Example deployment snippet:

```yaml
containers:
- image: rocm/k8s-device-plugin
  name: amdgpu-dp-cntr
  env:
  - name: CONFIG_FILE_PATH
    value: "/etc/amdgpu/config.yaml"
  volumeMounts:
  - name: config-volume
    mountPath: /etc/amdgpu
volumes:
- name: config-volume
  configMap:
    name: amdgpu-device-plugin-config
```

With a corresponding ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: amdgpu-device-plugin-config
  namespace: kube-system
data:
  config.yaml: |
    gpu:
      device_count: 2
```

### Essential Volume Mounts

These mounts are required for basic functionality:

| Mount Path | Purpose |
|------------|---------|
| `/var/lib/kubelet/device-plugins` | Required for device plugin registration with the Kubernetes kubelet |
| `/sys` | Required for GPU detection and topology information |

### Device Mounts

For GPU functionality, these device files must be accessible:

| Mount Path | Purpose |
|------------|---------|
| `/dev/kfd` | Kernel Fusion Driver interface, required for GPU compute workloads |
| `/dev/dri` | Direct Rendering Infrastructure, required for GPU access |

## Example Deployments

The repository contains example deployment configurations for different use cases.

### Basic Device Plugin (k8s-ds-amdgpu-dp.yaml)

A minimal deployment that exposes AMD GPUs to Kubernetes:

- Includes only the essential volume mounts
- Uses minimal security context settings
- Suitable for basic GPU workloads

[Download link](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml)

### Enhanced Device Plugin (k8s-ds-amdgpu-dp-health.yaml)

A more comprehensive deployment of the device plugin that includes additional volume mounts and privileged access for advanced features. This configuration includes:

- Additional volume mounts for `kfd` and `dri` devices
- A dedicated mount for metrics data
- Privileged execution context for direct hardware access

[Download link](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp-health.yaml)

### Node Labeller (k8s-ds-amdgpu-labeller.yaml)

Deploys the AMD GPU node labeller, which adds detailed GPU information as node labels:

- Requires access to `/sys` and `/dev` to gather GPU hardware information
- Creates Kubernetes node labels with details like VRAM size, compute units, etc.
- Helps with GPU-specific workload scheduling

The node labeller can expose labels such as:

- `amd.com/gpu.vram`: GPU memory size
- `amd.com/gpu.cu-count`: Number of compute units
- `amd.com/gpu.device-id`: Device ID of the GPU
- `amd.com/gpu.family`: GPU family/architecture
- `amd.com/gpu.product-name`: Product name of the GPU
- And others based on the passed arguments

[Download link](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-labeller.yaml)

## Resource Naming

The device plugin advertises AMD GPUs as the `amd.com/gpu` resource type. Pods can request this resource in their specifications to access AMD GPUs:

```yaml
resources:
  limits:
    amd.com/gpu: 1
```

## Security and Access Control

### Non-Privileged GPU Access

For secure workloads, it's recommended to run containers in non-privileged mode while still allowing GPU access. Based on testing with AMD ROCm containers, the following configuration provides reliable non-privileged GPU access:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-workload
spec:
  hostIPC: true
  containers:
  - name: gpu-container
    image: rocm/pytorch:latest
    resources:
      limits:
        amd.com/gpu: 1
    securityContext:
      # Run as non-privileged container
      privileged: false
      # Prevent privilege escalation
      allowPrivilegeEscalation: false
      # Allow necessary syscalls for GPU operations
      seccompProfile:
        type: Unconfined
```

#### Key Security Elements

- `privileged: false`: Ensures the container doesn't run with full host privileges
- `allowPrivilegeEscalation: false`: Prevents the process from gaining additional privileges
- `seccompProfile.type: Unconfined`: Allows necessary system calls for GPU operations
