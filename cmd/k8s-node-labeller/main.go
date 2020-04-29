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

	"github.com/RadeonOpenCompute/k8s-device-plugin/internal/pkg/amdgpu"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("amdgpu-node-labeller")

func createLabelPrefix(name string, experimental bool) string {
	var s string
	if experimental {
		s = "beta."
	} else {
		s = ""
	}

	return fmt.Sprintf("%samd.com/gpu.%s", s, name)
}

var reSizeInBytes = regexp.MustCompile(`size_in_bytes\s(\d+)`)
var reSimdCount = regexp.MustCompile(`simd_count\s(\d+)`)
var reSimdPerCu = regexp.MustCompile(`simd_per_cu\s(\d+)`)
var reDrmRenderMinor = regexp.MustCompile(`drm_render_minor\s(\d+)`)

var labelGenerators = map[string]func(map[string]map[string]int) map[string]string{
	"firmware": func(gpus map[string]map[string]int) map[string]string {
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
	"family": func(gpus map[string]map[string]int) map[string]string {
		counts := map[string]int{}

		for _, v := range gpus {
			fid, err := amdgpu.GetCardFamilyName(fmt.Sprintf("card%d", v["card"]))
			if err != nil {
				log.Error(err, "Fail to get card family name.")
				continue
			}
			counts[fid]++
		}

		pfx := createLabelPrefix("family", true)
		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
	"device-id": func(gpus map[string]map[string]int) map[string]string {
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

		pfx := createLabelPrefix("device-id", true)
		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
	"vram": func(gpus map[string]map[string]int) map[string]string {
		const bytePerMB = int64(1024 * 1024)
		counts := map[string]int{}

		pfx := createLabelPrefix("vram", true)
		for _, gpu := range gpus {
			// /sys/class/drm/card<card #>/device/mem_info_vram_total
			// size_in_bytes
			vramTotalPath := fmt.Sprintf("/sys/class/drm/card%d/device/mem_info_vram_total", gpu["card"])

			b, err := ioutil.ReadFile(vramTotalPath)
			if err != nil {
				log.Error(err, vramTotalPath)
				continue
			}
			vSize, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
			if err != nil {
				log.Error(err, "Error parsing size")
				continue
			}

			tmp := vSize / bytePerMB
			s := int(math.Round(float64(tmp) / 1024))
			counts[fmt.Sprintf("%dG", s)]++
		}

		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
	"simd-count": func(gpus map[string]map[string]int) map[string]string {
		counts := map[string]int{}

		propertiesPath := "/sys/class/kfd/kfd/topology/nodes/*/properties"
		var files []string
		var err error

		if files, err = filepath.Glob(propertiesPath); err != nil || len(files) == 0 {
			log.Error(err, "Fail to glob topology properties")
			return map[string]string{}
		}

		pfx := createLabelPrefix("simd-count", true)
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

		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
	"cu-count": func(gpus map[string]map[string]int) map[string]string {
		counts := map[string]int{}

		propertiesPath := "/sys/class/kfd/kfd/topology/nodes/*/properties"
		var files []string
		var err error

		if files, err = filepath.Glob(propertiesPath); err != nil || len(files) == 0 {
			log.Error(err, "Fail to glob topology properties")
			return map[string]string{}
		}

		pfx := createLabelPrefix("cu-count", true)
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

		results := make(map[string]string, len(counts))
		for k, v := range counts {
			results[fmt.Sprintf("%s.%s", pfx, k)] = strconv.Itoa(v)
		}
		return results
	},
}
var labelProperties = make(map[string]*bool, len(labelGenerators))

func generateLabels(lblProps map[string]*bool) map[string]string {
	results := make(map[string]string, len(labelGenerators))
	gpus := amdgpu.GetAMDGPUs()

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

var gitDescribe string

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "AMD GPU Node Labeller for Kubernetes\n")
		fmt.Fprintf(os.Stderr, "%s version %s\n", os.Args[0], gitDescribe)
		fmt.Fprintln(os.Stderr, "Usage:")
		flag.PrintDefaults()
	}

	for k := range labelGenerators {
		labelProperties[k] = flag.Bool(k, false, "Set this to label nodes with "+k+" properties")
	}

	flag.Parse()

	logf.SetLogger(zap.Logger(false))
	entryLog := log.WithName("entrypoint")

	// Setup a Manager
	entryLog.Info("setting up manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup a new controller to Reconciler Node labels
	entryLog.Info("Setting up controller")
	c, err := controller.New("amdgpu-node-labeller", mgr, controller.Options{
		Reconciler: &reconcileNodeLabels{client: mgr.GetClient(),
			log:    log.WithName("reconciler"),
			labels: generateLabels(labelProperties)},
	})
	if err != nil {
		entryLog.Error(err, "unable to set up individual controller")
		os.Exit(1)
	}

	// laballer only respond to event about the node it is on by matching hostname
	b, err := ioutil.ReadFile("/labeller/hostname")
	if err != nil {
		entryLog.Error(err, "Cannot read hostname")
	}
	hostname := strings.TrimSpace(string(b))

	pred := predicate.Funcs{
		// Create returns true if the Create event should be processed
		CreateFunc: func(e event.CreateEvent) bool {
			if hostname == e.Meta.GetName() {
				return true
			}
			return false
		},

		// Delete returns true if the Delete event should be processed
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},

		// Update returns true if the Update event should be processed
		UpdateFunc: func(e event.UpdateEvent) bool {
			return false
		},

		// Generic returns true if the Generic event should be processed
		GenericFunc: func(e event.GenericEvent) bool {
			//entryLog.Info("predicate generic triggered: ")
			return false
		},
	}

	// Watch Nodes and enqueue Nodes object key
	if err := c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{}, &pred); err != nil {
		entryLog.Error(err, "unable to watch Node")
		os.Exit(1)
	}

	entryLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
