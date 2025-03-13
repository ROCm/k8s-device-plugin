# Configuration Options

This document outlines the configuration options available for the AMD GPU device plugin for Kubernetes.

## Environment Variables

The device plugin can be configured using the following environment variables:

| Environment Variable | Type | Default | Description |
|-----|------|---------|-------------|
| `DP_DISABLE_HEALTHCHECK` | Boolean | `false` | Disables the GPU health checking feature when set to any value |
| `DP_HEALTHCHECK_INTERVAL` | Integer | `60` | Interval in seconds between GPU health checks |
| `AMD_GPU_DEVICE_COUNT` | Integer | Auto-detected | Number of AMD GPUs available on the node |
| `AMD_GPU_HEALTH_CHECK` | Boolean | `false` | Enables or disables the health check feature |

## Command-Line Flags

The device plugin supports the following command-line flags:

| Flag | Default | Description |
|-----|------|-------------|
| `--kubelet-url` | `http://localhost:10250` | The URL of the kubelet for device plugin registration |
| `--health-port` | `8080` | The port on which the health check service will listen |
| `--health-check-interval` | `30` | The interval at which health checks are performed (seconds) |

## Configuration File

You can also provide a configuration file in YAML format to customize the plugin's behavior:

```yaml
gpu:
  device_count: 2
  health_check: true
  health_check_interval: 30
```

## Volume Mounts

The device plugin requires certain volume mounts to function properly:

### Device Mounts

The plugin needs access to the host's device files, typically mounted at:

- `/dev/kfd`
- `/dev/dri`

### Kubelet Socket

The plugin needs access to the kubelet's device plugin socket, typically mounted at:

`/var/lib/kubelet/device-plugins`

## Example DaemonSet Configuration

Below is an example of the device plugin DaemonSet configuration:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: amdgpu-device-plugin-daemonset
  namespace: kube-system
spec:
  selector:
    matchLabels:
      name: amdgpu-dp-ds
  template:
    metadata:
      labels:
        name: amdgpu-dp-ds
    spec:
      containers:
      - name: amdgpu-dp-cntr
        image: rocm/k8s-device-plugin:latest
        env:
        - name: DP_DISABLE_HEALTHCHECK
          value: "false"
        - name: DP_HEALTHCHECK_INTERVAL
          value: "60"
        volumeMounts:
        - name: dev
          mountPath: /dev
        - name: dp
          mountPath: /var/lib/kubelet/device-plugins
      volumes:
      - name: dev
        hostPath:
          path: /dev
      - name: dp
        hostPath:
          path: /var/lib/kubelet/device-plugins
```

## Resource Naming

The device plugin advertises AMD GPUs as the `amd.com/gpu` resource type. Pods can request this resource in their specifications to access AMD GPUs.

## Node Labeling

When the device plugin runs on a node with AMD GPUs, it automatically advertises the available GPU resources. No additional node labeling is required.
