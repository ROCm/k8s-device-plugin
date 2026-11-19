# Resource allocation

## Overview

The [device plugin](https://github.com/ROCm/k8s-device-plugin) DaemonSet discovers AMD GPUs and makes them available to the Kubernetes cluster. Allocation logic determines which GPUs or partitions a job or pod receives when it requests them. The allocation logic can run an algorithm to determine which GPUs should be picked out of the available ones.

### Allocator package

The device plugin has an allocator package that can define multiple allocation policies. Each policy can follow a different algorithm to decide the allocation strategy based on system needs. Kubernetes and the kubelet perform the actual allocation of AMD GPUs. The allocation policy only decides the GPUs to be picked from the available GPUs for any given request.

### Best-effort allocation policy

The device plugin uses the `best-effort` policy as the default allocation policy. This policy chooses GPUs based on topology of the GPUs to ensure optimal affinity and better performance. During initialization, the device plugin calculates a score for every pair of GPUs and stores it in memory. This score is calculated based on the following criteria:

- Type of connectivity link between the pair. Most common AMD GPU deployments use either XGMI or PCIe links to connect the GPUs. XGMI connectivity offers better performance than PCIe connectivity. The score assigned for a pair connected using XGMI is lower than that of a pair connected using PCIe (a lower score is better).
- [NUMA affinity](https://rocm.blogs.amd.com/software-tools-optimization/affinity/part-1/README.html) of the GPU pair. A GPU pair in the same NUMA domain gets a lower score than a pair from different NUMA domains.
- For partitioned GPUs, partitions from the same GPU get a better score than partitions from different GPUs.

When a request for S GPUs arrives, the allocator calculates all subsets of size S out of available GPUs. For each set, the score is maintained (based on the criteria above). The allocator picks the set with the lowest score. The best-effort policy always tries to provide the best possible combination of GPUs out of the available GPU pool.

The allocator follows these rules for requests for X GPU partitions:

- The allocator tries to allocate all partitions from the same GPU if possible.
- In case there is a GPU with fewer available partitions that can accommodate the request, that GPU is preferred. This maximizes the utilization of GPUs already in use for other workloads and helps avoid fragmentation of unused GPUs.
- If more than one GPU is needed to accommodate the request, the allocator considers the topology (link type and NUMA affinity) as described above and generate all possible subsets. The subset with the lowest weight among the possible candidates is allocated.
