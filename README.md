# AMD GPU device plugin for Kubernetes

## Introduction
This is a [Kubernetes][k8s] [device plugin][dp] implementation that enables the registration of AMD GPU in a container cluster for compute workload.  With the approrpriate hardware and this plugin deployed in your Kubernetes cluster, you will be able to run jobs that require AMD GPU.

More information about [RadeonOpenCompute (ROCm)][rocm]


## Prerequisites
* [ROCm capable machines][sysreq]
* [kubeadm capable machines][kubeadm] (if you are using kubeadm to deploy your k8s cluster)
* [ROCm kernel][rock] ([Installation guide][rocminstall]) or latest AMD GPU Linux driver ([Installation guide][amdgpuinstall])
* A [Kubernetes deployment][k8sinstall] with the `DevicePlugins` [feature gate][k8sfg] set to true
* `--allow-privileged=true` for both kube-apiserver and kubelet (only needed if the device plugin is deployed via DaemonSet since the device plugin container requires privileged security context to access `/dev/kfd` for device health check)


## Limitations
* This is an early prototype/alpha.
* This plugin currently support device plugin API v1alpha only.  This means it will only work with k8s v1.9.\* because k8s v1.10+ have switched to v1beta1.

## Deployment
The device plugin needs to be run on all the nodes that are equipped with AMD GPU.  The simplist way of doing so is to create a Kubernete [DaemonSet][ds], which run a copy of a pod on all (or some) Nodes in the cluster.  We have a pre-built Docker image on [DockerHub][dhk8samdgpudp] that you can use for with your DaemonSet.  This repository also have a pre-defined yaml file named `k8s-ds-amdgpu-dp.yaml`.  You can create a DaemonSet in your Kubernetes cluster by running this command:
```
$ kubectl create -f k8s-ds-amdgpu-dp.yaml
```

If you want to enable the experimental device health check, please use `k8s-ds-amdgpu-dp-health.yaml` **after** `--allow-privileged=true` is set for kube-apiserver and kublet.

## Example workload
You can restrict work to a node with GPU by adding `resources.limits` to the pod definition.  An example pod definition is provided in `example/pod/alexnet-gpu.yaml`.  This pod runs the timing benchmark for AlexNet on AMD GPU and then go to sleep. You can create the pod by running:
```
$ kubectl create -f example/pod/alexnet-gpu.yaml
```
and then check the pod status by running
```
$ kubectl describe pods
```

After the pod is created and running, you can see the benchmark result by running:
```
$ kubectl logs alexnet-tf-gpu-pod alexnet-tf-gpu-container
```

For comparison, an example pod definition of running the same benchmark with CPU is provided in `example/pod/alexnet-cpu.yaml`.


## Notes
* This plugin uses [`go dep`][gd] for dependencies management
* Please consult the `Dockerfile` on how to build and use this plugin independent of a docker image

## TODOs
* Update plugin to support [device plugin][dp] API v1beta1
* Update ROCm documentation for kernel only install
* Add proper GPU health check (health check without `/dev/kfd` access.)

[ds]: https://kubernetes.io/docs/concepts/workloads/controllers/daemonset/
[dp]: https://kubernetes.io/docs/concepts/cluster-administration/device-plugins/
[rocm]: https://rocm.github.io/
[rock]: https://github.com/RadeonOpenCompute/ROCK-Kernel-Driver
[rocminstall]: http://rocm-documentation.readthedocs.io/en/latest/Installation_Guide/ROCk-kernel.html#rock-kernel
[amdgpuinstall]: https://support.amd.com/en-us/kb-articles/Pages/AMDGPU-PRO-Install.aspx
[sysreq]: http://rocm-documentation.readthedocs.io/en/latest/Installation_Guide/Installation-Guide.html#system-requirement
[gd]: https://github.com/golang/dep
[k8sfg]: https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/
[kubeadm]: https://kubernetes.io/docs/setup/independent/install-kubeadm/#before-you-begin
[k8sinstall]: https://kubernetes.io/docs/setup/independent/install-kubeadm
[k8s]: https://kubernetes.io
[dhk8samdgpudp]: https://hub.docker.com/r/rocm/k8s-device-plugin/
