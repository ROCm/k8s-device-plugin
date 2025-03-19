# Health Checks

## Overview

The AMD GPU Device Plugin for Kubernetes includes health check features that monitor the status of GPUs in your cluster. This document describes how to enable and utilize these health checks effectively.

## Health Check Implementation

The health check functionality continuously monitors the health status of AMD GPUs by:

1. Verifying access to `/dev/kfd` (Kernel Fusion Driver)
2. Checking GPU accessibility through ROCm system interfaces
3. Exposing health status through a dedicated socket service

### Health Check Parameters

The primary configuration parameter for health checks is:

- **-pulse**: Controls the polling interval for health checks in seconds (default: 10)

### Deployment with Health Checks

To deploy the AMD GPU device plugin with health check enabled:

1. **Deployment Configuration**: Use the health-check enabled DaemonSet configuration file (`k8s-ds-amdgpu-dp-health.yaml`) provided in the repository, which is specifically configured for health monitoring.

### Regular vs. Health-Check Enabled Deployment

There are two main deployment options:

- **Regular Deployment** (`k8s-ds-amdgpu-dp.yaml`): Basic deployment without health monitoring
- **Health-Check Enabled** (`k8s-ds-amdgpu-dp-health.yaml`): Includes additional configuration for the health check service

The health-check enabled version includes:

- Additional volume mounts for the metrics exporter
- Health check parameters in the container args
- Proper permissions to access device health information

### Example Configuration

Here is the specific configuration for enabling health checks in your DaemonSet:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: amdgpu-device-plugin
spec:
  template:
    spec:
      containers:
      - name: amdgpu-device-plugin
        image: rocm/k8s-device-plugin:latest
        args:
        - -pulse=10
        securityContext:
          privileged: true
          capabilities:
            add: ["SYS_ADMIN"]
        volumeMounts:
        - name: dev-kfd
          mountPath: /dev/kfd
        - name: dev-dri
          mountPath: /dev/dri
        - name: sys
          mountPath: /sys
        - name: metrics-exporter
          mountPath: /var/lib/amd-metrics-exporter/
      volumes:
      - name: dev-kfd
        hostPath:
          path: /dev/kfd
      - name: dev-dri
        hostPath:
          path: /dev/dri
      - name: sys
        hostPath:
          path: /sys
      - name: metrics-exporter
        emptyDir: {}
```

## Monitoring Health Status

You can monitor the health status of your GPUs using the metrics exposed by the health check service. This can be integrated with your existing monitoring solutions to provide alerts and insights into the GPU health.

```{note}
Add additional info here
```
