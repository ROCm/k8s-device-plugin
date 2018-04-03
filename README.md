# AMD GPU device plugin for Kubernetes

## Introduction
This is a [Kubernetes][k8s] [device plugin][dp] implementation that enables the registration of AMD GPU in a container cluster for compute workload.  With the approrpriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD GPU.

More information about [RadeonOpenCompute (ROCm)][rocm]


## Prerequisites
* [ROCm capable machines][sysreq]
* [kubeadm capable machines][kubeadm] (if you are using kubeadm to deploy your k8s cluster)
* [ROCm kernel][rock] ([Installation guide][rocminstall])
* A [Kubernetes deployment][k8sinstall] with the `DevicePlugins` [feature gate][k8sfg] set to true


## Limitations
* This is an early prototype **not meant for production deployment**.  This is **pre-alpha**.
* This plugin currently support device plugin API v1alpha only.  This means it will only work with k8s v1.8-v1.9.\* because k8s v1.10+ have switched to v1beta1.

## Notes
* This plugin uses [`go dep`][gd] for dependencies management


## TODOs
* Add `Dockerfile` to generate a canonical Docker image for plugin deployment
* Add deployment example with [DaemonSet][ds]
* Add pod usage example
* Update plugin to support [device plugin][dp] API v1beta1
* Update ROCm documentation for kernel only install
* Support multiple AMD GPU registration per node
* Add proper GPU health check

[ds]: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
[dp]: https://kubernetes.io/docs/concepts/cluster-administration/device-plugins/
[rocm]: https://rocm.github.io/
[rock]: https://github.com/RadeonOpenCompute/ROCK-Kernel-Driver
[rocminstall]: http://rocm-documentation.readthedocs.io/en/latest/Installation_Guide/Installation-Guide.html#
[sysreq]: http://rocm-documentation.readthedocs.io/en/latest/Installation_Guide/Installation-Guide.html#system-requirement
[gd]: https://github.com/golang/dep
[k8sfg]: https://kubernetes.io/docs/reference/feature-gates/
[kubeadm]: https://kubernetes.io/docs/setup/independent/install-kubeadm/#before-you-begin
[k8sinstall]: https://kubernetes.io/docs/setup/independent/install-kubeadm
[k8s]: https://kubernetes.io
