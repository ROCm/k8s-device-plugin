package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ROCm/k8s-device-plugin/internal/pkg/amdgpu"
	"github.com/ROCm/k8s-device-plugin/internal/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	log                      = logf.Log.WithName("amdgpu-node-labeller")
	gitDescribe              string
	allLabelKeys             = []string{}
	allExperimentalLabelKeys = []string{}
)

const (
	experimentalAMDPrefix = "beta.amd.com"
	amdPrefix             = "amd.com"
)

func init() {
	initLabelLists()
}

func initLabelLists() {
	// pre-generate all the available node labeller labels
	// these 2 lists will be used to clean up old labels on the node
	for name := range labelGenerators {
		allLabelKeys = append(allLabelKeys, createLabelPrefix(name, false))
		allExperimentalLabelKeys = append(allExperimentalLabelKeys, createLabelPrefix(name, true))
	}
}

func removeOldNodeLabels(node *corev1.Node) {
	if node == nil {
		return
	}
	// for the amd.com node labels
	// directly remove the old labels
	for _, label := range allLabelKeys {
		delete(node.Labels, label)
	}
	// for the beta.amd.com node labels
	// if it exists, both original label and counter label need to be removed, e.g.
	// beta.amd.com/gpu.family: AI
	// beta.amd.com/gpu.family.AI: "1"
	for _, label := range allExperimentalLabelKeys {
		if val, ok := node.Labels[label]; ok {
			delete(node.Labels, label)
			delete(node.Labels, fmt.Sprintf("%s.%s", label, val))
		}
	}
}

func createLabelPrefix(name string, experimental bool) string {
	var prefix string
	if experimental {
		prefix = experimentalAMDPrefix
	} else {
		prefix = amdPrefix
	}

	return fmt.Sprintf("%s/gpu.%s", prefix, name)
}

func createLabels(kind string, entries map[string]int) map[string]string {
	labels := make(map[string]string, len(entries))

	prefix := createLabelPrefix(kind, true)
	for k, v := range entries {
		labels[fmt.Sprintf("%s.%s", prefix, k)] = strconv.Itoa(v)
		if len(entries) == 1 {
			labels[prefix] = k
		}
	}

	prefix = createLabelPrefix(kind, false)
	for k, v := range entries {
		if len(entries) == 1 {
			labels[prefix] = k
		} else {
			labels[fmt.Sprintf("%s.%s", prefix, k)] = strconv.Itoa(v)
		}
	}

	return labels
}

var reSizeInBytes = regexp.MustCompile(`size_in_bytes\s(\d+)`)
var reSimdCount = regexp.MustCompile(`simd_count\s(\d+)`)
var reSimdPerCu = regexp.MustCompile(`simd_per_cu\s(\d+)`)
var reDrmRenderMinor = regexp.MustCompile(`drm_render_minor\s(\d+)`)

var labelGenerators = map[string]func(map[string]map[string]interface{}) map[string]string{
	"firmware": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}

		for _, v := range gpus {
			var featVersions map[string]uint32
			var fwVersions map[string]uint32

			featVersions, fwVersions, err := amdgpu.GetFirmwareVersions(fmt.Sprintf("card%d", v["card"]))
			if err != nil {
				log.Error(err, "Fail to get firmware versions")
				continue
			}

			for fw, ver := range featVersions {
				counts[fmt.Sprintf("%s.feat.%d", fw, ver)]++
			}
			for fw, ver := range fwVersions {
				counts[fmt.Sprintf("%s.fw.%d", fw, ver)]++
			}
		}

		pfx := createLabelPrefix("firmware", true)
		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
	"family": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}

		for _, v := range gpus {
			fid, err := amdgpu.GetCardFamilyName(fmt.Sprintf("card%d", v["card"]))
			if err != nil {
				log.Error(err, "Fail to get card family name.")
				continue
			}
			counts[fid]++
		}

		return createLabels("family", counts)
	},
	"driver-version": func(gpus map[string]map[string]interface{}) map[string]string {
		version := ""
		for _, v := range gpus {
			versionPath := fmt.Sprintf("/sys/class/drm/card%d/device/driver/module/version", v["card"])
			b, err := ioutil.ReadFile(versionPath)
			if err != nil {
				log.Error(err, versionPath)
				continue
			}
			version = strings.TrimSpace(string(b))
			break
		}

		pfx := createLabelPrefix("driver-version", false)
		return map[string]string{pfx: version}
	},
	"driver-src-version": func(gpus map[string]map[string]interface{}) map[string]string {
		version := ""
		for _, v := range gpus {
			versionPath := fmt.Sprintf("/sys/class/drm/card%d/device/driver/module/srcversion", v["card"])
			b, err := ioutil.ReadFile(versionPath)
			if err != nil {
				log.Error(err, versionPath)
				continue
			}
			version = strings.TrimSpace(string(b))
			break
		}

		pfx := createLabelPrefix("driver-src-version", false)
		return map[string]string{pfx: version}
	},
	"device-id": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}

		for _, v := range gpus {
			devidPath := fmt.Sprintf("/sys/class/drm/card%d/device/device", v["card"])
			b, err := ioutil.ReadFile(devidPath)
			if err != nil {
				log.Error(err, devidPath)
				continue
			}
			devid := strings.TrimSpace(string(b))
			if devid[0:2] == "0x" {
				devid = devid[2:]
			}
			counts[devid]++
		}

		return createLabels("device-id", counts)
	},
	"product-name": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}
		replacer := strings.NewReplacer(" ", "_", "(", "", ")", "")

		for _, v := range gpus {
			prodnamePath := fmt.Sprintf("/sys/class/drm/card%d/device/product_name", v["card"])
			b, err := ioutil.ReadFile(prodnamePath)
			if err != nil {
				log.Error(err, prodnamePath)
				continue
			}
			prodName := replacer.Replace(strings.TrimSpace(string(b)))
			if prodName == "" {
				continue
			}
			counts[prodName]++
		}

		return createLabels("product-name", counts)
	},
	"vram": func(gpus map[string]map[string]interface{}) map[string]string {
		const bytePerMB = int64(1024 * 1024)
		counts := map[string]int{}

		propertiesPath := "/sys/class/kfd/kfd/topology/nodes/*/properties"
		var files []string
		var err error

		if files, err = filepath.Glob(propertiesPath); err != nil || len(files) == 0 {
			log.Error(err, "Fail to glob topology properties")
			return map[string]string{}
		}

		for _, gpu := range gpus {
			// /sys/class/kfd/kfd/topology/nodes/*/properties

			for _, file := range files {
				render_minor, _ := amdgpu.ParseTopologyProperties(file, reDrmRenderMinor)

				if int(render_minor) != gpu["renderD"] {
					continue
				}
				parts := strings.Split(file, "/")
				nodeNumber := parts[len(parts)-2]

				vramTotalPath := fmt.Sprintf("/sys/class/kfd/kfd/topology/nodes/%s/mem_banks/0/properties", nodeNumber)

				vSize, err := amdgpu.ParseTopologyProperties(vramTotalPath, reSizeInBytes)
				if err != nil {
					log.Error(err, vramTotalPath)
					continue
				}

				tmp := vSize / bytePerMB
				s := int(math.Round(float64(tmp) / 1024))
				counts[fmt.Sprintf("%dG", s)]++
				break
			}
		}

		return createLabels("vram", counts)
	},
	"simd-count": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}

		propertiesPath := "/sys/class/kfd/kfd/topology/nodes/*/properties"
		var files []string
		var err error

		if files, err = filepath.Glob(propertiesPath); err != nil || len(files) == 0 {
			log.Error(err, "Fail to glob topology properties")
			return map[string]string{}
		}

		for _, gpu := range gpus {
			// /sys/class/kfd/kfd/topology/nodes/*/properties
			// simd_count

			for _, file := range files {
				render_minor, _ := amdgpu.ParseTopologyProperties(file, reDrmRenderMinor)

				if int(render_minor) != gpu["renderD"] {
					continue
				}

				s, e := amdgpu.ParseTopologyProperties(file, reSimdCount)
				if e != nil {
					log.Error(e, "Error parsing simd-count")
					continue
				}

				counts[fmt.Sprintf("%d", s)]++
				break
			}
		}

		return createLabels("simd-count", counts)
	},
	"cu-count": func(gpus map[string]map[string]interface{}) map[string]string {
		counts := map[string]int{}

		propertiesPath := "/sys/class/kfd/kfd/topology/nodes/*/properties"
		var files []string
		var err error

		if files, err = filepath.Glob(propertiesPath); err != nil || len(files) == 0 {
			log.Error(err, "Fail to glob topology properties")
			return map[string]string{}
		}

		for _, gpu := range gpus {
			// /sys/class/kfd/kfd/topology/nodes/*/properties
			// simd_count / simd_per_cu

			for _, file := range files {
				render_minor, _ := amdgpu.ParseTopologyProperties(file, reDrmRenderMinor)

				if int(render_minor) != gpu["renderD"] {
					continue
				}

				s, e := amdgpu.ParseTopologyProperties(file, reSimdCount)
				if e != nil {
					log.Error(e, "Error parsing simd-count")
					continue
				}
				c, e := amdgpu.ParseTopologyProperties(file, reSimdPerCu)
				if e != nil || c == 0 {
					log.Error(e, fmt.Sprintf("Error parsing simd-per-cu %d", c))
					continue
				}

				counts[fmt.Sprintf("%d", s/c)]++
				break
			}
		}

		return createLabels("cu-count", counts)
	},
	"compute-memory-partition": func(gpus map[string]map[string]interface{}) map[string]string {
		partitionCountMap := amdgpu.UniquePartitionConfigCount(gpus)
		isHomogeneous := amdgpu.IsHomogeneous(gpus)
		if isHomogeneous {
			for partitionType, count := range partitionCountMap {
				if count > 0 {
					pfx := createLabelPrefix("compute-memory-partition", false)
					return map[string]string{pfx: partitionType}
				}
			}
		}
		return map[string]string{}
	},
	"compute-partitioning-supported": func(gpus map[string]map[string]interface{}) map[string]string {
		val := strconv.FormatBool(amdgpu.IsComputePartitionSupported())
		pfx := createLabelPrefix("compute-partitioning-supported", false)
		return map[string]string{pfx: val}
	},
	"memory-partitioning-supported": func(gpus map[string]map[string]interface{}) map[string]string {
		val := strconv.FormatBool(amdgpu.IsMemoryPartitionSupported())
		pfx := createLabelPrefix("memory-partitioning-supported", false)
		return map[string]string{pfx: val}
	},
	"mode": func(gpus map[string]map[string]interface{}) map[string]string {
		labels := make(map[string]string, 2)
		labels[createLabelPrefix("mode", true)] = types.Container
		labels[createLabelPrefix("mode", false)] = types.Container
		return labels
	},
}

var labelProperties = make(map[string]*bool, len(types.SupportedLabels))

func generateLabels(lblProps map[string]*bool, driverType string) map[string]string {
	switch driverType {
	case types.Container:
		return generateContainerLabels(lblProps)
	case types.VFPassthrough:
		return generateSRIOVVFLabels(lblProps)
	case types.PFPassthrough:
		return generatePFLabels(lblProps)
	default:
		// Best effort: try container first, then VF, then PF.
		labels := generateContainerLabels(lblProps)
		if len(labels) == 0 {
			labels = generateSRIOVVFLabels(lblProps)
		}
		if len(labels) == 0 {
			labels = generatePFLabels(lblProps)
		}
		return labels
	}
}

func generateContainerLabels(lblProps map[string]*bool) map[string]string {
	results := make(map[string]string, len(labelGenerators))
	gpus := amdgpu.GetAMDGPUs()

	if len(gpus) == 0 {
		log.Info("No AMD GPUs found, skipping label generation")
		return results
	}

	for l, f := range labelGenerators {
		if !*lblProps[l] {
			continue
		}

		for k, v := range f(gpus) {
			results[k] = v
		}
	}

	return results
}

func generateSRIOVVFLabels(lblProps map[string]*bool) map[string]string {
	results := make(map[string]string)

	// Retrieve VF mapping from the amdgpu package
	vfMapping, err := amdgpu.GetVFMapping()
	if err != nil || len(vfMapping) < 1 {
		return results
	}

	// Driver details
	gimVersion, gimSrcVersion, err := amdgpu.GetGIMVersions()
	if err != nil {
		return results
	}

	if *lblProps["driver-version"] {
		results[createLabelPrefix("driver-version", false)] = gimVersion
	}

	if *lblProps["driver-src-version"] {
		results[createLabelPrefix("driver-src-version", false)] = gimSrcVersion
	}

	if *lblProps["mode"] {
		results[createLabelPrefix("mode", false)] = types.VFPassthrough
		results[createLabelPrefix("mode", true)] = types.VFPassthrough
	}

	if *lblProps["device-id"] {
		// Aggregate counts
		deviceMap := map[string]int{}
		for _, vfList := range vfMapping {
			for _, vfInfo := range vfList {
				deviceMap[vfInfo.ID]++
			}
		}

		for k, v := range createLabels("device-id", deviceMap) {
			results[k] = v
		}
	}

	return results
}

func generatePFLabels(lblProps map[string]*bool) map[string]string {
	results := make(map[string]string)

	// Retrieve PF mapping from the amdgpu package
	pfMapping, err := amdgpu.GetPFMapping()
	if err != nil || len(pfMapping) < 1 {
		return results
	}

	if *lblProps["mode"] {
		results["amd.com/gpu.mode"] = types.PFPassthrough
	}

	if *lblProps["device-id"] {
		// Aggregate counts
		deviceMap := map[string]int{}
		for _, pfList := range pfMapping {
			for _, pfInfo := range pfList {
				deviceMap[pfInfo.ID]++
			}
		}

		for k, v := range createLabels("device-id", deviceMap) {
			results[k] = v
		}
	}

	return results
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "AMD GPU Node Labeller for Kubernetes\n")
		fmt.Fprintf(os.Stderr, "%s version %s\n", os.Args[0], gitDescribe)
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
	}

	var driverType string
	flag.StringVar(&driverType, "driver_type", "", "Driver type to use: container, vf-passthrough, or pf-passthrough")

	for _, k := range types.SupportedLabels {
		labelProperties[k] = flag.Bool(k, false, "Set this to label nodes with "+k+" properties")
	}

	flag.Parse()

	logf.SetLogger(zap.New())
	entryLog := log.WithName("entrypoint")

	// Setup a Manager
	entryLog.Info("setting up manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		// disable the metrics server
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup a new controller to Reconciler Node labels
	entryLog.Info("Setting up controller")
	c, err := controller.New("amdgpu-node-labeller", mgr, controller.Options{
		Reconciler: &reconcileNodeLabels{client: mgr.GetClient(),
			log:    log.WithName("reconciler"),
			labels: generateLabels(labelProperties, driverType)},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up individual controller")
		os.Exit(1)
	}

	// laballer only respond to event about the node it is on by matching hostname
	hostname := os.Getenv("DS_NODE_NAME")

	pred := predicate.TypedFuncs[*corev1.Node]{
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.TypedCreateEvent[*corev1.Node]) bool {
			if hostname == e.Object.GetName() {
				return true
			}
			return false
		},

		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.TypedDeleteEvent[*corev1.Node]) bool {
			return false
		},

		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.TypedUpdateEvent[*corev1.Node]) bool {
			return false
		},

		// Generic returns true if the Generic event should be processed
		GenericFunc: func(e event.TypedGenericEvent[*corev1.Node]) bool {
			//entryLog.Info("predicate generic triggered: ")
			return false
		},
	}

	// Watch Nodes and enqueue Nodes object key
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Node{}, &handler.TypedEnqueueRequestForObject[*corev1.Node]{}, &pred)); err != nil {
		entryLog.Error(err, "unable to watch Node")
		os.Exit(1)
	}

	entryLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
