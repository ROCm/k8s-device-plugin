# Resource Allocation

## Overview

[Device Plugin](https://github.com/ROCm/k8s-device-plugin) daemon set discovers and makes the AMD GPUs available to Kubernetes cluster. Allocation logic determines which set of GPUs/resources are allocated when a Job/Pod requests for them. The allocation logic can run an alogrithm to determine which GPUs should be picked out of the available ones.

### Allocator package

Device Plugin has allocator package where we can define multiple policies on how the allocation should be done. Each policy can follow a different algorithm to decide the allocation strategy based on system needs. Actual allocation of AMD GPUs is done by Kubernetes and Kubelet. The allocation policy only decides the GPUs to be picked from the available GPUs for any given request.

### Best-effort Allocation Policy

Currently we use ```best-effort``` policy as the default allocation policy. This policy choses GPUs based on topology of the GPUs to ensure optimal affinity and better performance. During initialization phase, Device Plugin calculates a score for every pair of GPUs and stores it in memory. This score is calculated based on below criteria:
- Type of connectivity link between the pair. Most common AMD GPU deployments use either XGMI or PCIE links to connect the GPUs. ```XGMI``` connectivity offers better performance than PCIE connectivity. The score assigned for a pair connected using XGMI is lower than that of a pair connected using PCIE(lower score is better)
- [NUMA affinity](https://rocm.blogs.amd.com/software-tools-optimization/affinity/part-1/README.html) of the GPU pair. GPU pair that is part of same NUMA domain get lower score than pair from different NUMA domains.
- For scenarios that involve partitioned GPUs, partitions from same GPU are assigned better score than partitions from different GPUs.

When an allocation request for size S comes, the allocator calculates all subsets of size S out of available GPUs. For each set, the score is maintained(based on above criteria). Set with lowest score is picked for allocation. At any given time, best-effort policy tries to provide best possible combination of GPUs out of the avilable GPU pool.
