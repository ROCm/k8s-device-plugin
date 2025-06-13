/**
 * Copyright 2025 Advanced Micro Devices, Inc.  All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
**/

package types

var SupportedLabels = []string{"mode", "firmware", "family", "driver-version", "driver-src-version", "device-id", "product-name", "vram", "simd-count", "cu-count", "compute-memory-partition", "compute-partitioning-supported", "memory-partitioning-supported"}

// Command line parameters
const (
	// Time between health check polling in seconds
	CmdLinePulse = "pulse"

	// Driver type to use: container, vf-passthrough, or pf-passthrough
	CmdLineDriverType = "driver_type"

	// Resource strategy to be used
	CmdLineResNamingStrategy = "resource_naming_strategy"
)

// Resource Naming Strategies
const (
	// Naming strategy single
	ResourceNamingStrategySingle = "single"

	// Naming strategy mixed
	ResourceNamingStrategyMixed = "mixed"
)

// Driver types for device plugin, node labeler
const (
	// Container workloads
	Container = "container"

	// SRIOV VF based workloads
	VFPassthrough = "vf-passthrough"

	// PF Passthrough based workloads
	PFPassthrough = "pf-passthrough"
)

// AMDGPU constants
const (
	// VFIO Driver path
	VFIODriverPath = "/sys/bus/pci/drivers/vfio-pci"

	// VFIO Driver name
	VFIODriverName = "vfio-pci"

	// GIM Driver Path for SRIOV VFs
	AMDGIMDriverPath = "/sys/bus/pci/drivers/gim"

	// AMDGIMModulePath module path for gim driver
	AMDGIMModulePath = "/sys/module/gim"

	// AMD GIM Driver name
	AMDGIMDriverName = "gim"

	// PCI Device Path to discover AMD GPU PFs/VFs
	PCIDevicePath = "/sys/bus/pci/devices/"

	// PCI GPU Prefix env variable for VF/PF allocation
	PCIGpuPrefix = "PCI_RESOURCE_AMD_COM"

	// AMD PCI Vendor ID
	AMDVendorID = "0x1002"

	// Device Type reported to kubelet
	DeviceTypeGPU = "gpu"
)
