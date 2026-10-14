/**
 * Copyright 2026 Advanced Micro Devices, Inc.  All rights reserved.
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

// Package naming decides which resource name each GPU is advertised under.
// ResourceList and ResourceName both answer from the same rule, so the set of
// advertised names and the bucket a device falls in cannot disagree.
package naming

import (
	"fmt"
	"sort"
	"strings"
)

type Strategy string

// Resource names these produce: single -> gpu, mixed -> the partition style,
// product -> the marketing name from amdgpu.ids.
const (
	StrategySingle  Strategy = "single"
	StrategyMixed   Strategy = "mixed"
	StrategyProduct Strategy = "product"
)

// DefaultResource is used when a strategy has nothing more specific to say.
const DefaultResource = "gpu"

// maxResourceNameLen is the Kubernetes limit on the name after "amd.com/".
const maxResourceNameLen = 63

func ParseStrategy(s string) (Strategy, error) {
	switch s {
	case string(StrategySingle):
		return StrategySingle, nil
	case string(StrategyMixed):
		return StrategyMixed, nil
	case string(StrategyProduct):
		return StrategyProduct, nil
	default:
		return "", fmt.Errorf("invalid resource naming strategy: %s", s)
	}
}

// SanitizeResourceName maps a marketing name onto a valid resource name, or ""
// if nothing is left. "AMD Instinct MI250X / MI250" has a slash a name may not.
func SanitizeResourceName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}

	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return truncate(out)
}

// truncate cuts to the Kubernetes length limit without leaving a leading or
// trailing separator, which would make the name invalid: a qualified name may
// contain "-", "." and "_" but must begin and end alphanumeric.
func truncate(name string) string {
	if len(name) > maxResourceNameLen {
		name = name[:maxResourceNameLen]
	}
	return strings.Trim(name, "-._")
}

// partitionStyle returns "" for a GPU that does not support partitioning.
func partitionStyle(device map[string]interface{}) string {
	compute, _ := device["computePartitionType"].(string)
	memory, _ := device["memoryPartitionType"].(string)
	if compute == "" || memory == "" {
		return ""
	}
	return compute + "_" + memory
}

// ResourceName returns the name, without the "amd.com/" prefix, that one device
// is advertised under. products is consulted only by StrategyProduct.
func ResourceName(device map[string]interface{}, strategy Strategy, products ProductIndex) string {
	switch strategy {
	case StrategyMixed:
		if style := partitionStyle(device); style != "" {
			return style
		}
		return DefaultResource
	case StrategyProduct:
		deviceID, _ := device["deviceID"].(string)
		revisionID, _ := device["revisionID"].(string)
		name := products.Name(deviceID, revisionID)
		if name == "" {
			name = DefaultResource
		}
		// A CPX partition and a whole GPU are not interchangeable. This holds
		// for a device the table does not list too, which would otherwise pool
		// the two under a bare "gpu" -- worse than the mixed strategy an
		// operator migrated away from, and silently so.
		if style := partitionStyle(device); style != "" {
			return truncate(name + "-" + SanitizeResourceName(style))
		}
		return name
	default:
		return DefaultResource
	}
}

// partitionStyles returns the distinct styles present. More than one is a
// heterogeneous node.
func partitionStyles(devices map[string]map[string]interface{}) map[string]struct{} {
	styles := map[string]struct{}{}
	for _, device := range devices {
		if style := partitionStyle(device); style != "" {
			styles[style] = struct{}{}
		}
	}
	return styles
}

// ResourceList returns the sorted, de-duplicated names to advertise.
func ResourceList(devices map[string]map[string]interface{}, strategy Strategy, products ProductIndex) ([]string, error) {
	if len(devices) == 0 {
		return nil, nil
	}

	if strategy == StrategySingle && len(partitionStyles(devices)) > 1 {
		return nil, fmt.Errorf("Partitions of different styles across GPUs in a node is not supported with single strategy. Please start device plugin with mixed strategy")
	}

	seen := map[string]struct{}{}
	resources := []string{}
	for _, device := range devices {
		name := ResourceName(device, strategy, products)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		resources = append(resources, name)
	}

	sort.Strings(resources)
	return resources, nil
}
