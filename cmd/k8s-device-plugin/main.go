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

// Kubernetes (k8s) device plugin to enable registration of AMD GPU to a container cluster
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/amdgpu"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/hwloc"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/manager"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/types"
	"github.com/golang/glog"
)

var gitDescribe string

func main() {
	var devImpl types.DeviceImpl

	versions := [...]string{
		"AMD GPU device plugin for Kubernetes",
		fmt.Sprintf("%s version %s", os.Args[0], gitDescribe),
		fmt.Sprintf("%s", hwloc.GetVersions()),
	}

	flag.Usage = func() {
		for _, v := range versions {
			fmt.Fprintf(os.Stderr, "%s\n", v)
		}
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
	}
	var pulse int
	var driverType string
	var resourceNamingStrategy string
	flag.IntVar(&pulse, types.CmdLinePulse, 0, "time between health check polling in seconds.  Set to 0 to disable.")
	flag.StringVar(&driverType, types.CmdLineDriverType, "", "Driver type to use: container, vf-passthrough, or pf-passthrough")
	flag.StringVar(&resourceNamingStrategy, types.CmdLineResNamingStrategy, "single", "Resource strategy to be used: single or mixed")
	// this is also needed to enable glog usage in dpm
	flag.Parse()

	validateFlags := func(pulse int, driverType, resourceNamingStrategy string) error {
		if pulse < 0 {
			return fmt.Errorf("pulse must be a non-negative integer, got %d", pulse)
		}
		if driverType != "" && driverType != types.Container && driverType != types.VFPassthrough && driverType != types.PFPassthrough {
			return fmt.Errorf("invalid driver_type provided: %s, supported values are container, vf-passthrough, or pf-passthrough", driverType)
		}
		if resourceNamingStrategy != types.ResourceNamingStrategySingle && resourceNamingStrategy != types.ResourceNamingStrategyMixed {
			return fmt.Errorf("invalid resource_naming_strategy provided: %s, supported values are single or mixed", resourceNamingStrategy)
		}
		return nil
	}
	err := validateFlags(pulse, driverType, resourceNamingStrategy)
	if err != nil {
		glog.Errorf("%v", err)
		os.Exit(1)
	}

	for _, v := range versions {
		glog.Infof("%s", v)
	}

	initParams := map[string]interface{}{
		types.CmdLineResNamingStrategy: resourceNamingStrategy,
	}

	deviceImplList := []struct {
		name    string
		creator func(params map[string]interface{}) (types.DeviceImpl, error)
	}{
		{types.Container, amdgpu.NewGPUKFDImpl},    // Container-based implementation using ROCm/KFD
		{types.VFPassthrough, amdgpu.NewGPUVFImpl}, // SR-IOV VF passthrough implementation
		{types.PFPassthrough, amdgpu.NewGPUPFImpl}, // PF passthrough implementation
	}

	if driverType != "" {
		// Use the specified driver type
		for _, impl := range deviceImplList {
			if impl.name == driverType {
				devImpl, err = impl.creator(initParams)
				if err != nil {
					glog.Errorf("Error instantiating driver type %s: %v", driverType, err)
					os.Exit(1)
				}
				break
			}
		}
	} else {
		// Try implementations in order if no driver type is provided
		for _, impl := range deviceImplList {
			devImpl, err = impl.creator(initParams)
			if err == nil {
				break
			}
			glog.Warningf("%s implementation failed: %v. Trying next...", impl.name, err)
		}
	}

	// Start a new plugin manager regardless of implementation status
	mgr := manager.NewPluginManager(pulse, devImpl)
	mgr.Run()
}
