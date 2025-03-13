# Node Labelling

Node labelling allows Kubernetes administrators to categorize and manage nodes in a cluster based on their capabilities, such as the presence of AMD GPUs. This document covers both automated and manual approaches to node labelling.

## Automated Node Labelling

### Overview

The AMD GPU Kubernetes Node Labeller (`k8s-node-labeller`) automatically detects AMD GPUs and applies appropriate labels to Kubernetes nodes. This tool leverages the [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) framework and runs as a DaemonSet to ensure all nodes with AMD GPUs are properly labelled.

### Detected GPU Properties

The automated node labeller detects and labels the following GPU properties:

- Device ID
- Product Name
- Driver Version
- VRAM Size
- SIMD Count
- Compute Unit count
- GPU Family information
- Firmware and Feature Versions

These properties enable fine-grained pod scheduling based on specific GPU capabilities.

### Deployment

To deploy the automated node labeller, use the pre-configured DaemonSet provided in the repository (`k8s-ds-amdgpu-labeller.yaml`):

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: amd-gpu-node-labeller
  namespace: kube-system
spec:
  selector:
    matchLabels:
      name: amd-gpu-node-labeller
  template:
    metadata:
      labels:
        name: amd-gpu-node-labeller
    spec:
      containers:
      - name: amd-gpu-node-labeller
        image: amd-gpu-node-labeller:latest
        resources:
          limits:
            cpu: 100m
            memory: 128Mi
          requests:
            cpu: 100m
            memory: 128Mi
        securityContext:
          privileged: true
      tolerations:
      - key: node.kubernetes.io/not-ready
        operator: Exists
        effect: NoExecute
      - key: node.kubernetes.io/unreachable
        operator: Exists
        effect: NoExecute
```

Apply the DaemonSet configuration using the following command:

```bash
kubectl apply -f amd-gpu-node-labeller-daemonset.yaml
```

### Manual Node Labelling

If you prefer to label nodes manually, you can use the `kubectl` command-line tool. Here’s how to do it:

1. **Identify the Node**: First, identify the node that you want to label. You can list all nodes in your cluster with the following command:

```bash
kubectl get nodes
```

1. **Apply the Label**: Use the following command to label the node. Replace `<node-name>` with the name of your node and `<label-key>=<label-value>` with your desired label.

```bash
kubectl label nodes <node-name> <label-key>=<label-value>
```

For example, to label a node named `gpu-node-1` with the label `gpu=true`, you would run:

```bash
kubectl label nodes gpu-node-1 gpu=true
```

1. **Verify the Label**: To verify that the label has been applied successfully, you can describe the node:

```bash
kubectl describe node <node-name>
```

 Look for the `Labels` section in the output to confirm that your label is present.

## Best Practices

- **Use Descriptive Labels**: Choose label keys and values that clearly describe the node's capabilities. For example, use `gpu=amd` for nodes with AMD GPUs.
- **Consistent Labeling**: Ensure that all nodes with similar capabilities are labeled consistently to avoid confusion during scheduling.
- **Combine Labels**: You can apply multiple labels to a single node to provide more context about its capabilities. For example, a node could be labeled with both `gpu=true` and `vendor=amd`.

## Example

Here’s an example of labeling multiple nodes:

```bash
kubectl label nodes gpu-node-1 gpu=true
kubectl label nodes gpu-node-2 gpu=true
kubectl label nodes gpu-node-3 gpu=true
```

After applying these commands, all specified nodes will have the label `gpu=true`, indicating that they are equipped with AMD GPUs.
