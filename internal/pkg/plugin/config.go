/**
 * Copyright 2018 Advanced Micro Devices, Inc.  All rights reserved.
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

package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// GPUConfig holds GPU-specific configuration fields.
type GPUConfig struct {
	// Replicas is the number of virtual devices to create per physical GPU.
	// Default is 1 (no overcommit). Must be >= 1.
	Replicas int `yaml:"replicas"`
}

// Config is the top-level configuration loaded from a YAML file.
type Config struct {
	GPU GPUConfig `yaml:"gpu"`
}

// LoadConfig reads and parses a YAML configuration file.
// If path is empty, a default config (replicas=1) is returned.
// Returns an error if replicas is < 1.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		GPU: GPUConfig{
			Replicas: 1,
		},
	}

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %v", path, err)
	}

	// Default replicas to 1 if not specified (zero value after unmarshal)
	if cfg.GPU.Replicas == 0 {
		cfg.GPU.Replicas = 1
	}

	if cfg.GPU.Replicas < 1 {
		return nil, fmt.Errorf("invalid gpu.replicas value %d: must be >= 1", cfg.GPU.Replicas)
	}

	return cfg, nil
}
