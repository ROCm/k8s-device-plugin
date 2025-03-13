# Monitoring

Monitoring the AMD GPU device plugin is essential for ensuring that your workloads are running efficiently and that the GPUs are being utilized correctly. This document outlines the available monitoring capabilities and integration options.

## Available Metrics

The AMD GPU device plugin provides basic health and status metrics that can be integrated with monitoring systems like Prometheus. These metrics primarily focus on:

- **Device Health Status**: Indicates whether a GPU is functioning properly
- **Device Allocation Status**: Shows which GPUs are allocated to pods
- **Device Plugin Status**: Overall operational status of the plugin

Note that detailed hardware metrics like utilization percentages, memory usage, temperature, and power consumption are not directly exposed by the device plugin itself but can be collected using AMD's ROCm-SMI tools in a separate metrics exporter.

## Setting Up Monitoring

To monitor the AMD GPU device plugin:

1. **Enable Health Checks**: Deploy the device plugin with health checks enabled (see [Health Checks](health-checks.md) for details).

2. **Configure Prometheus**: Set up Prometheus to scrape the metrics endpoint exposed by the device plugin.

   Example configuration specific to the AMD GPU device plugin:

```yaml
scrape_configs:
  - job_name: 'amd-gpu-device-plugin'
    kubernetes_sd_configs:
      - role: endpoints
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_name]
        action: keep
        regex: amd-gpu-device-plugin
```

3. **Visualize Metrics**: Use Grafana or another visualization tool to create dashboards that display the metrics collected from the AMD GPU device plugin. You can create graphs for device health status, allocation status, and plugin status.

## Alerts

Setting up alerts based on the metrics collected can help you proactively manage your GPU resources. Consider configuring alerts for:

- Device health status indicating a malfunction.
- Unexpected changes in device allocation status.
- Plugin status showing operational issues.