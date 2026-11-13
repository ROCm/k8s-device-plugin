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
| `-pulse` | `0` | Time between health check polling in seconds. Set to 0 to disable. |
| `-resource_naming_strategy` | `single` | Resource naming strategy used for Kubernetes resource reporting: `single`, `mixed` or `product`. |
| `-occupancy_check` | `false` | Fail allocation when KFD reports VRAM still allocated to a requested GPU, or when that occupancy cannot be read. Does not signal processes. |
| `-occupancy_reclaim` | `false` | Signal orphaned container processes holding KFD VRAM on a requested GPU, log each one, and continue only after the GPU is free. Live workloads and processes whose identity cannot be confirmed are not signaled. Implies `-occupancy_check`. Requires `hostPID: true` and `CAP_KILL`. |
| `-occupancy_reclaim_timeout` | `60s` | How long to wait for VRAM counters to return to zero after signaling. Must be greater than 0. |
| `-occupancy_reclaim_cooldown` | `60s` | Minimum time a GPU stays unschedulable after reclaim fails. It stays unschedulable after that while KFD VRAM remains allocated. |

### KFD VRAM reclaim

`-occupancy_check` reads `/sys/class/kfd/kfd/proc/<pid>/vram_<gpu_id>` during Allocate and PreStartContainer. `<gpu_id>` is the value in `/sys/class/kfd/kfd/topology/nodes/<node>/gpu_id` for the GPU kubelet selected. A missing counter means that process has no allocation on the GPU. If any counter is nonzero, or if the KFD process tree cannot be read, allocation fails closed.

`-occupancy_reclaim` uses that same scan, then logs every occupying process (pid, command name, and VRAM bytes). It sends SIGTERM, and SIGKILL after a short grace, only when every one of these holds: the pid is not 1 and not the device plugin, `/proc/<pid>/stat` starttime still matches the process seen in the KFD scan, the parent pid is 1 (the process was reparented after its parent exited), and the cgroup belongs to a Kubernetes pod or a container scope (`kubepods`, `cri-containerd`, `crio-`, `docker-`, or `libpod-`), not the runtime daemon itself. The signal goes through a pidfd after that starttime is checked again, so a recycled pid is not signaled. A live container process, a host service, or a process whose identity cannot be read is left alone. If one of those still holds VRAM, reclaim fails closed. The GPU is ready only when every counter reads as zero. KFD directory names are host PIDs, so the device plugin has to run with `hostPID: true` and `CAP_KILL` for those signals to reach the right processes. Both the static DaemonSet and the Helm chart leave that off unless reclaim is explicitly enabled. In Helm, set `dp.occupancyReclaim.enabled=true`; that turns on `hostPID`, passes `-occupancy_reclaim`, and adds `CAP_KILL` to the chart's otherwise unprivileged container. The static manifest's container is already privileged, so enabling `hostPID` and passing the flag is enough there.

If the counters are still nonzero when `-occupancy_reclaim_timeout` expires, or the RPC is canceled while VRAM remains allocated, Allocate returns an error before kubelet checkpoints the GPU and ListAndWatch marks that GPU unhealthy. A cancellation that finds the GPU already free does not quarantine it and does not send any further signals. On a request for several GPUs, the first error stops the scan so later GPUs are not signaled. The following allocation prefers another healthy GPU. The quarantined GPU becomes schedulable again only after `-occupancy_reclaim_cooldown` has elapsed and a later scan shows no VRAM allocated. Cooldown expiry alone does not return a GPU that is still occupied, so reclaim is not repeated against the same processes. The plugin rechecks that on its own, so a GPU that has both aged out and gone idle does not stay unhealthy until the next allocation request. Kubelet cancels PreStartContainer after 30s. Allocate, which assigns the GPU the first time, waits for the full deadline unless its own context is canceled.

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

Exposing GPU Partition related through Node Labeller:

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

To customize the way device plugin reports gpu resources to kubernetes as allocatable k8s resources, use the `single`, `mixed` or `product` resource naming strategy flag mentioned above (--resource_naming_strategy)

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

### Product

In `product` mode, the device plugin reports all gpu's under a name which matches their GPU product, and
the partition style where the GPU is partitioned. This mode is supported for both homogeneous nodes
and heterogeneous nodes. It is for clusters whose nodes hold more than one GPU product, where
`amd.com/gpu` cannot express which product a workload received, so per-product quota and scheduling are
not possible.

A node which has 4 MI300X and 4 MI210 GPUs, none of them partitioned, will report its resources as:

```bash
amd.com/gpu-mi300x-spx-nps1: 4
amd.com/gpu-mi210: 4
```

The partition style is part of the name because a CPX partition and a whole GPU are not
interchangeable. If those MI300X GPUs were in CPX-NPS4, the node would report:

```bash
amd.com/gpu-mi300x-cpx-nps4: 32
```

MI210 does not support partitioning, so it carries no style. Partitions of a GPU are reported under
the product of the GPU they are a partition of.

The product name comes from `amdgpu.ids`, the device-id/revision-id to product-name table maintained in
this repository at `cmd/k8s-node-labeller/amdgpu.ids` and installed into the image at
`/usr/share/libdrm/amdgpu.ids`. The plugin reads each GPU's PCI device and revision id from
`/sys` and looks the pair up in that table itself, so naming a GPU needs no access to the card's
DRM device node. `AMD Instinct MI300X` becomes `gpu-mi300x`: the name is lowercased, the brand and
category words `amd`, `ati`, `instinct`, `radeon`, `graphics` and `series` are dropped, and
characters Kubernetes does not permit in a resource name are replaced with `-`.

Two products whose names shorten to the same thing both keep the category words instead, so that one
resource never covers two products: `AMD Radeon 540` and `AMD Radeon 540 Series` become
`amd.com/gpu-radeon-540` and `amd.com/gpu-radeon-540-series`. `amd` and `ati` are dropped even then,
since a resource under `amd.com/` does not need to repeat the vendor. This is decided across the whole
table rather than from the GPUs on a node, so every node names a given product identically whatever
hardware it holds. A GPU with no entry in the table is reported as `amd.com/gpu`, which means
supporting a new product is an `amdgpu.ids` update rather than a code change.

Some rows name more than one product, separated by `/`, because one device id covers both and the
driver cannot tell them apart — `AMD Instinct MI250X / MI250` is one such. The alternatives are
joined with `_`, as `amd.com/gpu-mi250x_mi250`, rather than resolved to one of them: the separator
does not mean the same thing on every row, so reading one alternative as the answer would assert
something the table does not say. `-` remains the word separator, which keeps the two levels legible
against each other.

The plugin exits at startup if `product` is requested and `/usr/share/libdrm/amdgpu.ids` cannot be
read, rather than silently reporting every GPU as `amd.com/gpu`.

- If `resource_naming_strategy` is not passed using the flag, then device plugin will internally default to `single` resource naming strategy. This maintains backwards compatibility with earlier release of device plugin with reported resource name of `amd.com/gpu`

- If a node has GPUs which do not support partitioning, such as MI210, then the GPUs are reported under resource name `amd.com/gpu` under the `single` and `mixed` strategies. Under `product` they are reported under their product name with no partition style appended.

- A GPU with no `amdgpu.ids` entry still carries its partition style under `product`, as `amd.com/gpu-cpx-nps4`, so that a CPX partition and a whole GPU are never pooled under a bare `amd.com/gpu`.

- Changing the strategy on a running node renames its resources. Pods that requested the old name become unschedulable, because the node no longer advertises it. Drain the node before changing the strategy, and move workloads to the new name before uncordoning it.

- Upgrading while **keeping** `mixed` can also move capacity, on one node shape: a node whose GPUs all share one partition style but which also holds GPUs that do not support partitioning. Earlier releases reported every GPU on such a node under the single style — 4 MI300X in `spx_nps1` plus 4 MI210 reported `amd.com/spx_nps1: 8`. They are now separated, as `amd.com/spx_nps1: 4` and `amd.com/gpu: 4`. A Deployment or ResourceQuota pinned to the old count becomes unschedulable. Drain such a node before upgrading, or raise the request against the split names afterwards.

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

```yaml
resources:
  limits:
    amd.com/gpu-mi300x-cpx-nps4: 1
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
  securityContext:
    # ROCm framework images run as root. Stated explicitly so the Pod does not
    # inherit an unreviewed default.
    runAsNonRoot: false
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
- `runAsNonRoot: false`: ROCm framework images run as root. Setting it explicitly records that as a reviewed choice rather than an inherited default
