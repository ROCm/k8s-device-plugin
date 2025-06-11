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
| `--resource_naming_strategy` | `single` | Resource Naming strategy chosen for k8s resource reporting. |
| `--driver_type` | `""` | GPU operational mode: `container`, `vf-passthrough`, or `pf-passthrough`. When empty (default), automatic mode detection is used. |

### Driver Type and Operational Modes

The `--driver_type` flag controls how the device plugin operates and what types of GPU resources it exposes:

- **`container` (default when ROCm/AMDGPU driver detected)**: Standard GPU usage for containerized applications using `/dev/kfd` and `/dev/dri` devices
- **`vf-passthrough`**: Virtual Function passthrough for KubeVirt VMs using SR-IOV and VFIO. Requires [AMD MxGPU GIM driver](https://github.com/amd/MxGPU-Virtualization)
- **`pf-passthrough`**: Physical Function passthrough for KubeVirt VMs using VFIO. No special driver requirements beyond VFIO

When `--driver_type` is not specified or set to empty string, the device plugin automatically detects the operational mode by inspecting the system configuration (presence of `/dev/kfd`, VF symlinks, driver bindings, etc.) in this order - `container`, `vf-passthrough` and `pf-passthrough`.

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

The required device mounts depend on the operational mode:

#### Container Mode (Standard GPU Access)
| Mount Path | Purpose |
|------------|---------|
| `/dev/kfd` | Kernel Fusion Driver interface, required for GPU compute workloads |
| `/dev/dri` | Direct Rendering Infrastructure, required for GPU access |

#### VF/PF Passthrough Mode (KubeVirt)
| Mount Path | Purpose |
|------------|---------|
| `/dev/vfio/<iommu_group_id>` | VFIO device file for the specific IOMMU group containing the VF or PF |
| `/dev/vfio/vfio` | VFIO container device, required for all VFIO operations |

**IOMMU Groups**: In VF/PF passthrough modes, devices are allocated by IOMMU groups rather than individual GPUs. Each IOMMU group represents a set of devices that must be assigned together for security and isolation. The device plugin:
- For VF mode: Discovers VFs and maps them to their IOMMU groups. The number of resources equals the number of unique IOMMU groups containing VFs
- For PF mode: Discovers PFs bound to `vfio-pci` and maps them to their IOMMU groups. Each IOMMU group containing PFs becomes one resource

Resource names depend on the naming strategy:
- **Single strategy**: All resources reported as `amd.com/gpu`
- **Mixed strategy**: VF resources as `amd.com/gpu_vf`, PF resources as `amd.com/gpu_pf`

The device plugin sets the environment variable `PCI_RESOURCE_AMD_COM_<RESOURCE_NAME>` (e.g., `PCI_RESOURCE_AMD_COM_GPU`) containing comma-separated PCI addresses of the allocated VFs or PFs.

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

#### Container Mode Labels

For standard containerized GPU workloads, the node labeller can expose labels such as:

- `amd.com/gpu.mode`: Operational mode (`container`)
- `amd.com/gpu.vram`: GPU memory size
- `amd.com/gpu.cu-count`: Number of compute units
- `amd.com/gpu.device-id`: Device ID of the GPU
- `amd.com/gpu.family`: GPU family/architecture
- `amd.com/gpu.product-name`: Product name of the GPU
- `amd.com/gpu.driver-version`: ROCm/AMDGPU driver version

#### KubeVirt Passthrough Mode Labels

For VF and PF passthrough modes (KubeVirt integration), these labels are applied:

**Common Labels:**
- `amd.com/gpu.mode`: Operational mode (`vf-passthrough`, or `pf-passthrough`)
- `amd.com/gpu.device-id`: Device ID (VF device ID for VF mode, PF device ID for PF mode)
- `amd.com/gpu.device-id.<DEVICE_ID>`: Count of devices with specific device ID

**VF Passthrough Specific Labels:**
- `amd.com/gpu.driver-version`: GIM driver version (when managed by the operator)

**Note**: The `amd.com/gpu.driver-version` label is not available in `pf-passthrough` mode as no special driver is required.

#### GPU Partition Labels

As part of the arguments passed while starting node labeller, these flags can be passed to expose partition labels:
- compute-partitioning-supported
- memory-partitioning-supported
- compute-memory-partition

These 3 labels have these respective possible values
- `amd.com/compute-partitioning-supported`: ["true", "false"]
- `amd.com/memory-partitioning-supported`: ["true", "false"]
- `amd.com/compute-memory-partition`: ["spx_nps1", "cpx_nps1" ,"cpx_nps4", ...]

[Download link](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-labeller.yaml)

## Resource Naming Strategy

To customize the way device plugin reports gpu resources to kubernetes as allocatable k8s resources, use the `single` or `mixed` resource naming strategy flag mentioned above (--resource_naming_strategy).

The resource naming strategy affects all operational modes:

### Container Mode
- **Single**: All GPUs reported as `amd.com/gpu`
- **Mixed**: GPUs reported by partition type (e.g., `amd.com/cpx_nps4`, `amd.com/spx_nps1`)

### VF/PF Passthrough Modes
- **Single**: All resources reported as `amd.com/gpu`
- **Mixed**: Resources reported as `amd.com/gpu_vf` (VF passthrough) or `amd.com/gpu_pf` (PF passthrough)

Before understanding each strategy, please note the definition of homogeneous and heterogeneous nodes

Homogeneous node: A node whose gpu's follow the same compute-memory partition style 
    -> Example: A node of 8 GPU's where all 8 GPU's are following CPX-NPS4 partition style

Heterogeneous node: A node whose gpu's follow different compute-memory partition styles
    -> Example: A node of 8 GPU's where 5 GPU's are following SPX-NPS1 and 3 GPU's are following CPX-NPS1

### Single

In `single` mode, the device plugin reports all gpu's (regardless of whether they are whole gpu's or partitions of a gpu) under the resource name `amd.com/gpu`
This mode is supported for homogeneous nodes but not supported for heterogeneous nodes

A node which has 8 GPUs where all GPUs are not partitioned will report its resources as:

```bash
amd.com/gpu: 8
```

A node which has 8 GPUs where all GPUs are partitioned using CPX-NPS4 style will report its resources as:

```bash
amd.com/gpu: 64
```

### Mixed

In `mixed` mode, the device plugin reports all gpu's under a name which matches its partition style.
This mode is supported for both homogeneous nodes and heterogeneous nodes

A node which has 8 GPUs which are all partitioned using CPX-NPS4 style will report its resources as:

```bash
amd.com/cpx_nps4: 64
```

A node which has 8 GPUs where 5 GPU's are following SPX-NPS1 and 3 GPU's are following CPX-NPS1 will report its resources as:

```bash
amd.com/spx_nps1: 5
amd.com/cpx_nps1: 24
``` 

- If `resource_naming_strategy` is not passed using the flag, then device plugin will internally default to `single` resource naming strategy. This maintains backwards compatibility with earlier release of device plugin with reported resource name of `amd.com/gpu`

- If a node has GPUs which do not support partitioning, such as MI210, then the GPUs are reported under resource name `amd.com/gpu` regardless of the resource naming strategy

Pods can request the resource as per the naming style in their specifications to access AMD GPUs:

```yaml
resources:
  limits:
    amd.com/gpu: 1
```

```yaml
resources:
  limits:
    amd.com/cpx_nps4: 1
```

**VF/PF Passthrough Examples:**
```yaml
# Single strategy - both VF and PF use amd.com/gpu
resources:
  limits:
    amd.com/gpu: 1
```

```yaml
# Mixed strategy - VF passthrough
resources:
  limits:
    amd.com/gpu_vf: 1
```

```yaml
# Mixed strategy - PF passthrough
resources:
  limits:
    amd.com/gpu_pf: 1
```

## KubeVirt Integration

The AMD GPU device plugin supports integration with [KubeVirt](https://kubevirt.io/) for GPU passthrough to virtual machines. This enables VMs running in Kubernetes to access AMD GPUs directly.

### Prerequisites

- KubeVirt installed in the cluster
- Host nodes configured for IOMMU and VFIO support
- For VF passthrough: AMD MxGPU GIM driver installed and VFs created
- For PF passthrough: GPUs bound to `vfio-pci` driver

### VF Passthrough Configuration

For Virtual Function passthrough using SR-IOV:

1. **Host Setup**: Install AMD MxGPU GIM driver and configure SR-IOV VFs
2. **Device Plugin**: Use `--driver_type=vf-passthrough` or let auto-detection discover VF mode
3. **Resource Allocation**: VFs are grouped by IOMMU groups and advertised as `amd.com/gpu` (single strategy) or `amd.com/gpu_vf` (mixed strategy)

**MI210 Specific Constraints**: Due to XGMI fabric architecture, VFs can only be assigned to VMs in combinations of 1, 2, 4, or 8 VFs per hive (typically 4 VFs per hive).

### PF Passthrough Configuration

For Physical Function passthrough:

1. **Host Setup**: Bind AMD GPU PFs to `vfio-pci` driver
2. **Device Plugin**: Use `--driver_type=pf-passthrough` or let auto-detection discover PF mode
3. **Resource Allocation**: PFs are grouped by IOMMU groups and advertised as `amd.com/gpu` (single strategy) or `amd.com/gpu_pf` (mixed strategy)

### KubeVirt VM Example

Example KubeVirt VirtualMachine requesting GPU passthrough:

**Single Resource Naming Strategy:**
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: gpu-vm
spec:
  template:
    spec:
      domain:
        devices:
          hostDevices:
          - deviceName: amd.com/gpu
            name: gpu1
        resources:
          requests:
            amd.com/gpu: 1
      nodeSelector:
        amd.com/gpu.mode: vf-passthrough  # or pf-passthrough
```

**Mixed Resource Naming Strategy:**
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: gpu-vm-vf
spec:
  template:
    spec:
      domain:
        devices:
          hostDevices:
          - deviceName: amd.com/gpu_vf
            name: gpu1
        resources:
          requests:
            amd.com/gpu_vf: 1
      nodeSelector:
        amd.com/gpu.device-id: "0x74b5"  # MI300X VF device ID
---
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: gpu-vm-pf
spec:
  template:
    spec:
      domain:
        devices:
          hostDevices:
          - deviceName: amd.com/gpu_pf
            name: gpu1
        resources:
          requests:
            amd.com/gpu_pf: 1
      nodeSelector:
        amd.com/gpu.device-id: "0x74a1"  # MI300X PF device ID
```

### Verification

After VM deployment, verify GPU passthrough inside the VM:

```bash
# Inside the VM
lspci | grep -i amd
```

For VF passthrough, you should see VF devices. For PF passthrough, you should see the full PF devices.

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
