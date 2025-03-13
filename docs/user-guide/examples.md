# Example Workloads

This document provides example workloads and configurations for using the AMD GPU device plugin in Kubernetes.

## Basic GPU Pod Example

Here's a simple example of a pod requesting an AMD GPU:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-pod-example
spec:
  containers:
  - name: gpu-container
    image: rocm/tensorflow:latest
    command: ["python3", "-c", "import tensorflow as tf; print(tf.config.list_physical_devices('GPU'))"]
    resources:
      limits:
        amd.com/gpu: 1  # Request 1 AMD GPU
```

## Multiple GPU Example

For workloads that require multiple GPUs:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: multi-gpu-pod
spec:
  containers:
  - name: multi-gpu-container
    image: rocm/tensorflow:latest
    command: ["python3", "-c", "import tensorflow as tf; print(tf.config.list_physical_devices('GPU'))"]
    resources:
      limits:
        amd.com/gpu: 2  # Request 2 AMD GPUs
```

## ROCm Example with TensorFlow

Here's a more complete example using TensorFlow with ROCm:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: rocm-tensorflow-mnist
spec:
  template:
    spec:
      containers:
      - name: tensorflow
        image: rocm/tensorflow:latest
        command: ["/bin/bash", "-c"]
        args:
          - python3 -c '
```
