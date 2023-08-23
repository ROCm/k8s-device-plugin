# amd-gpu

![Version: 0.8.1](https://img.shields.io/badge/Version-0.8.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.25.2.3](https://img.shields.io/badge/AppVersion-1.25.2.3-informational?style=flat-square)

A Helm chart for deploying Kubernetes AMD GPU device plugin

**Homepage:** <https://github.com/RadeonOpenCompute/k8s-device-plugin>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Kenny Ho <Kenny.Ho@amd.com> |  |  |

## Source Code

* <https://github.com/RadeonOpenCompute/k8s-device-plugin>

## Requirements

Kubernetes: `>= 1.18.0-0`

| Repository | Name | Version |
|------------|------|---------|
| https://kubernetes-sigs.github.io/node-feature-discovery/charts | node-feature-discovery | >= 0.8.1-0 |

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| dp.image.repository | string | `"docker.io/rocm/k8s-device-plugin"` |  |
| dp.image.tag | string | `"1.25.2.3"` |  |
| dp.podAnnotations | object | `{}` |  |
| dp.podLabels | object | `{}` |  |
| dp.resources | object | `{}` |  |
| imagePullSecrets | list | `[]` |  |
| labeller.enabled | bool | `false` |  |
| lbl.image.repository | string | `"docker.io/rocm/k8s-device-plugin"` |  |
| lbl.image.tag | string | `"labeller-1.25.2.3"` |  |
| lbl.podAnnotations | object | `{}` |  |
| lbl.podLabels | object | `{}` |  |
| lbl.resources | object | `{}` |  |
| namespace | string | `"kube-system"` |  |
| nfd.enabled | bool | `false` |  |
| node_selector."feature.node.kubernetes.io/pci-0300_1002.present" | string | `"true"` |  |
| node_selector."kubernetes.io/arch" | string | `"amd64"` |  |
| securityContext.allowPrivilegeEscalation | bool | `false` |  |
| securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| tolerations[0].key | string | `"CriticalAddonsOnly"` |  |
| tolerations[0].operator | string | `"Exists"` |  |

