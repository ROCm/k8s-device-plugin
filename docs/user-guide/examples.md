# Example Workloads

This document provides example workloads and configurations for using the AMD GPU device plugin in Kubernetes.

## Basic GPU Pod Example

This example demonstrates how to run a basic PyTorch workload on an AMD GPU. The pod creates simple tensors on the GPU and performs basic addition operations to verify GPU functionality. Since this is a job-like workload that runs once and completes, we set `restartPolicy: Never` to prevent the pod from restarting after completion.

Here's a simple example of a pod requesting an AMD GPU:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: pytorch-gpu-pod-example
spec:
  restartPolicy: Never
  containers:
  - name: gpu-container
    image: rocm/pytorch:latest
    command:
    - python3
    - "-c"
    - |
      import torch
      if torch.cuda.is_available():
        print(f"GPU is available. Device count: {torch.cuda.device_count()}")
        print(f"Device name: {torch.cuda.get_device_name(0)}")
        x = torch.ones(3, 3, device='cuda')
        y = torch.ones(3, 3, device='cuda') * 2
        z = x + y
        print(f"Result of tensor addition on GPU: {z}")
      else:
        print("No GPU available.")
    resources:
      limits:
        amd.com/gpu: 1  # Request 1 AMD GPU
```

To run the example:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/pytorch.yaml
```

Check the output with:

```bash
kubectl logs pytorch-gpu-pod-example
```

This example manifest is available for download here: [https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/pytorch.yaml](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/pytorch.yaml)

## Multiple GPU Example

This example shows how to utilize multiple GPUs in a JAX application. It performs parallel matrix multiplications across both GPUs using JAX's pmap functionality for distributed computation.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: jax-multi-gpu-pod
spec:
  restartPolicy: Never
  containers:
  - name: multi-gpu-container
    image: rocm/jax:latest
    command:
    - /bin/bash
    - "-c"
    - |
      python3 -c "
      import jax
      import jax.numpy as jnp
      print('Available JAX devices:', jax.devices())

      # Create data to process in parallel
      n_devices = jax.device_count()
      print(f'Number of devices: {n_devices}')

      # Create matrices for each device
      x = jnp.ones((n_devices, 1000, 1000))
      y = jnp.ones((n_devices, 1000, 1000))

      # Define computation to run in parallel
      @jax.pmap
      def parallel_matmul(a, b):
          return jnp.matmul(a, b)

      # Run computation in parallel across GPUs
      result = parallel_matmul(x, y)

      print(f'Parallel computation complete across {n_devices} devices')
      print('Result shape:', result.shape)
      print('Device mapping:', jax.devices())
      "
    resources:
      limits:
        amd.com/gpu: 2  # Request 2 AMD GPUs
```

To run the example:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-non-privileged.yaml
```

Check the output with:

```bash
kubectl logs jax-multigpu-pod
```

This example manifest is available for download here: [https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-mult-gpu.yaml](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-mult-gpu.yaml)

## Non-privileged Pod with GPU Access Example

This example demonstrates the same JAX example as above, running as a non-privileged container configuration for enhanced security.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: jax-non-privileged-multi-gpu-pod
spec:
  restartPolicy: Never
  hostIPC: true
  containers:
  - name: jax-multi-gpu-container
    image: rocm/jax:latest
    command:
    - python3
    - "-c"
    - |
      import jax
      import jax.numpy as jnp
      print('Available JAX devices:', jax.devices())

      # Create data to process in parallel
      n_devices = jax.device_count()
      print(f'Number of devices: {n_devices}')

      # Create matrices for each device
      x = jnp.ones((n_devices, 1000, 1000))
      y = jnp.ones((n_devices, 1000, 1000))

      # Define computation to run in parallel
      @jax.pmap
      def parallel_matmul(a, b):
          return jnp.matmul(a, b)

      # Run computation in parallel across GPUs
      result = parallel_matmul(x, y)

      print(f'Parallel computation complete across {n_devices} devices')
      print('Result shape:', result.shape)
      print('Device mapping:', jax.devices())
    resources:
      limits:
        amd.com/gpu: 2  # Request 2 AMD GPUs
    securityContext:
      privileged: false
      allowPrivilegeEscalation: false
      seccompProfile:
        type: Unconfined
```

To run the example:

```bash
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-non-privileged.yaml
```

Check the output with:

```bash
kubectl logs jax-non-privileged-multi-gpu-pod
```

This example manifest is available for download here: [https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-non-privileged.yaml](https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/example/pod/jax-non-privileged.yaml)
