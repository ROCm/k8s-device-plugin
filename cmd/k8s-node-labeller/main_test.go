package main

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	expectedAllLabelKeys = map[string]bool{
		"amd.com/gpu.family":                         true,
		"amd.com/gpu.driver-version":                 true,
		"amd.com/gpu.driver-src-version":             true,
		"amd.com/gpu.firmware":                       true,
		"amd.com/gpu.device-id":                      true,
		"amd.com/gpu.product-name":                   true,
		"amd.com/gpu.vram":                           true,
		"amd.com/gpu.simd-count":                     true,
		"amd.com/gpu.cu-count":                       true,
		"amd.com/gpu.compute-memory-partition":       true,
		"amd.com/gpu.compute-partitioning-supported": true,
		"amd.com/gpu.memory-partitioning-supported":  true,
	}
	expectedAllExperimentalLabelKeys = map[string]bool{
		"beta.amd.com/gpu.family":                         true,
		"beta.amd.com/gpu.driver-version":                 true,
		"beta.amd.com/gpu.driver-src-version":             true,
		"beta.amd.com/gpu.firmware":                       true,
		"beta.amd.com/gpu.device-id":                      true,
		"beta.amd.com/gpu.product-name":                   true,
		"beta.amd.com/gpu.vram":                           true,
		"beta.amd.com/gpu.simd-count":                     true,
		"beta.amd.com/gpu.cu-count":                       true,
		"beta.amd.com/gpu.compute-memory-partition":       true,
		"beta.amd.com/gpu.compute-partitioning-supported": true,
		"beta.amd.com/gpu.memory-partitioning-supported":  true,
	}
)

func TestInitLabelLists(t *testing.T) {
	labelMap := map[string]bool{}
	for _, label := range allLabelKeys {
		labelMap[label] = true
	}
	if !reflect.DeepEqual(labelMap, expectedAllLabelKeys) {
		t.Errorf("failed to get expected all labels during init, got %+v, expect %+v", labelMap, expectedAllLabelKeys)
	}
	experimentalLabelMap := map[string]bool{}
	for _, label := range allExperimentalLabelKeys {
		experimentalLabelMap[label] = true
	}
	if !reflect.DeepEqual(experimentalLabelMap, expectedAllExperimentalLabelKeys) {
		t.Errorf("failed to get expected all experimental labels during init, got %+v, expect %+v", labelMap, expectedAllLabelKeys)
	}
}

func TestRemoveOldNodeLabels(t *testing.T) {
	testCases := []struct {
		inputNode    *corev1.Node
		expectLabels map[string]string
	}{
		{
			inputNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"amd.com/gpu.cu-count":                          "104",
						"amd.com/gpu.device-id":                         "740f",
						"amd.com/gpu.driver-version":                    "6.10.5",
						"amd.com/gpu.family":                            "AI",
						"amd.com/gpu.product-name":                      "Instinct_MI210",
						"amd.com/gpu.simd-count":                        "416",
						"amd.com/gpu.vram":                              "64G",
						"beta.amd.com/gpu.cu-count":                     "104",
						"beta.amd.com/gpu.cu-count.104":                 "1",
						"beta.amd.com/gpu.device-id":                    "740f",
						"beta.amd.com/gpu.device-id.740f":               "1",
						"beta.amd.com/gpu.family":                       "HPC",
						"beta.amd.com/gpu.family.HPC":                   "1",
						"beta.amd.com/gpu.product-name":                 "Instinct_MI300X",
						"beta.amd.com/gpu.product-name.Instinct_MI300X": "1",
						"beta.amd.com/gpu.simd-count":                   "416",
						"beta.amd.com/gpu.simd-count.416":               "1",
						"beta.amd.com/gpu.vram":                         "64G",
						"beta.amd.com/gpu.vram.64G":                     "1",
						"dummyLabel1":                                   "1",
						"dummyLabel2":                                   "2",
					},
				},
			},
			expectLabels: map[string]string{
				"dummyLabel1": "1",
				"dummyLabel2": "2",
			},
		},
		{
			inputNode: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"amd.com/cpu":    "true",
						"amd.com/gpu":    "true",
						"amd.com/mi300x": "true",
						"dummyLabel1":    "1",
						"dummyLabel2":    "2",
					},
				},
			},
			expectLabels: map[string]string{
				"amd.com/cpu":    "true",
				"amd.com/gpu":    "true",
				"amd.com/mi300x": "true",
				"dummyLabel1":    "1",
				"dummyLabel2":    "2",
			},
		},
	}

	for _, tc := range testCases {
		removeOldNodeLabels(tc.inputNode)
		if !reflect.DeepEqual(tc.inputNode.Labels, tc.expectLabels) {
			t.Errorf("failed to get expected node labels after removing old labels, got %+v, expect %+v", tc.inputNode.Labels, tc.expectLabels)
		}
	}
}
