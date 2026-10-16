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

package naming

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// vendoredIDs is the table the Dockerfiles install at IDsPath.
const vendoredIDs = "../../../cmd/k8s-node-labeller/amdgpu.ids"

func writeIDs(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "amdgpu.ids")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadProductIndex(t *testing.T) {
	path := writeIDs(t, `# List of AMDGPU IDs
#
# device_id,	revision_id,	product_name
1.0.0
74A1,	00,	AMD Instinct MI300X
74B5,	00,	AMD Instinct MI300X VF
740F,	02,	AMD Instinct MI210
malformed line without commas
`)

	index, err := LoadProductIndex(path)
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}

	want := map[ProductID]string{
		{DeviceID: "74A1", RevisionID: "00"}: "gpu-mi300x",
		{DeviceID: "74B5", RevisionID: "00"}: "gpu-mi300x-vf",
		{DeviceID: "740F", RevisionID: "02"}: "gpu-mi210",
	}
	for id, resource := range want {
		if got := index.Name(id.DeviceID, id.RevisionID); got != resource {
			t.Errorf("Name(%q, %q) = %q, want %q", id.DeviceID, id.RevisionID, got, resource)
		}
	}
	if got := index.Name("FFFF", "00"); got != "" {
		t.Errorf("Name of an absent device = %q, want \"\"", got)
	}
	// The revision is part of the key: 66A1/02 and 66A1/06 are different parts.
	if got := index.Name("740F", "00"); got != "" {
		t.Errorf("Name of a known device at an unknown revision = %q, want \"\"", got)
	}
}

// sysfs spells the ids "0x74a1"; amdgpu.ids spells them "74A1".
func TestLoadProductIndexAcceptsEitherIDSpelling(t *testing.T) {
	index, err := LoadProductIndex(writeIDs(t, "1.0.0\n74A1,\t00,\tAMD Instinct MI300X\n"))
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}
	for _, spelling := range [][2]string{{"74A1", "00"}, {"0x74a1", "0x00"}, {" 74a1 ", " 0X00 "}} {
		if got := index.Name(spelling[0], spelling[1]); got != "gpu-mi300x" {
			t.Errorf("Name(%q, %q) = %q, want %q", spelling[0], spelling[1], got, "gpu-mi300x")
		}
	}
}

func TestLoadProductIndexFallsBackOnCollision(t *testing.T) {
	path := writeIDs(t, `1.0.0
67DF,	C4,	AMD Radeon 540
67DF,	C5,	AMD Radeon 540 Series
74A1,	00,	AMD Instinct MI300X
`)

	index, err := LoadProductIndex(path)
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}

	a, b := index.Name("67DF", "C4"), index.Name("67DF", "C5")
	if a == b {
		t.Errorf("colliding products share the resource name %q", a)
	}
	// Category words come back to separate them; the vendor word does not.
	if a != "gpu-radeon-540" || b != "gpu-radeon-540-series" {
		t.Errorf("collision fallback = %q and %q", a, b)
	}
	for _, got := range []string{a, b} {
		if strings.Contains(got, "amd") {
			t.Errorf("fallback %q repeats the vendor already in amd.com/", got)
		}
	}
	if got := index.Name("74A1", "00"); got != "gpu-mi300x" {
		t.Errorf("an uncontested product should stay short, got %q", got)
	}
}

// A "/" is a disjunction libdrm cannot resolve, and it does not mean the same
// thing on every row, so the alternatives are joined with "_" rather than
// resolved into a claim the table does not make. "-" stays the word separator,
// so the two levels remain legible against each other.
func TestLoadProductIndexJoinsAlternativesWithUnderscore(t *testing.T) {
	path := writeIDs(t, `1.0.0
740C,	01,	AMD Instinct MI250X / MI250
66A1,	02,	AMD Instinct MI60 / MI50
6798,	00,	AMD Radeon R9 200 / HD 7900 Series
69AF,	CF,	AMD Radeon RX Vega (mobile / OEM)
`)

	index, err := LoadProductIndex(path)
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}

	want := map[ProductID]string{
		{DeviceID: "740C", RevisionID: "01"}: "gpu-mi250x_mi250",
		{DeviceID: "66A1", RevisionID: "02"}: "gpu-mi60_mi50",
		{DeviceID: "6798", RevisionID: "00"}: "gpu-r9-200_hd-7900",
		// Not a product separator at all, which is why nothing here tries to
		// read one alternative as the answer.
		{DeviceID: "69AF", RevisionID: "CF"}: "gpu-rx-vega-mobile_oem",
	}
	for id, resource := range want {
		if got := index.Name(id.DeviceID, id.RevisionID); got != resource {
			t.Errorf("Name(%q, %q) = %q, want %q", id.DeviceID, id.RevisionID, got, resource)
		}
	}
}

// A name of nothing but category words must not reduce to bare "gpu", which
// already means "not in the table".
func TestLoadProductIndexKeepsAnAllCategoryName(t *testing.T) {
	index, err := LoadProductIndex(writeIDs(t, "1.0.0\n6902,\t00,\tAMD Radeon Series\n"))
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}
	if got := index.Name("6902", "00"); got != "gpu-radeon-series" {
		t.Errorf("Name(6902, 00) = %q, want %q", got, "gpu-radeon-series")
	}
}

// A row that cannot derive any name at all is left out rather than indexed as
// the default, which a caller could not tell from an untabled device.
func TestLoadProductIndexDropsAnNamelessRow(t *testing.T) {
	index, err := LoadProductIndex(writeIDs(t, "1.0.0\n74A1,\t00,\tAMD Instinct MI300X\n0001,\t00,\tAMD\n"))
	if err != nil {
		t.Fatalf("LoadProductIndex: %v", err)
	}
	if got := index.Name("0001", "00"); got != "" {
		t.Errorf("Name(0001, 00) = %q, want \"\"", got)
	}
}

func TestLoadProductIndexRejectsUnusableTable(t *testing.T) {
	if _, err := LoadProductIndex(filepath.Join(t.TempDir(), "absent.ids")); err == nil {
		t.Error("a missing table should be an error")
	}
	if _, err := LoadProductIndex(writeIDs(t, "# comments only\n1.0.0\n")); err == nil {
		t.Error("a table with no product names should be an error")
	}
}

// A contested name is the one kind that moves when an unrelated entry is added,
// so pin the set: a refresh that contests a new name fails here rather than
// renaming a resource on a live cluster.
func TestVendoredIDsContestedSetIsPinned(t *testing.T) {
	index, marketing, _ := loadVendored(t)

	want := map[string]bool{
		"AMD Radeon 540":             true, // gpu-radeon-540
		"AMD Radeon 540 Series":      true, // gpu-radeon-540-series
		"AMD Radeon HD 8500M":        true, // gpu-radeon-hd-8500m
		"AMD Radeon HD 8500M Series": true, // gpu-radeon-hd-8500m-series
		"AMD Radeon R7 M260":         true, // gpu-radeon-r7-m260
		"AMD Radeon R7 M260 Series":  true, // gpu-radeon-r7-m260-series
		"AMD Radeon RX 470 Graphics": true, // gpu-radeon-rx-470-graphics
		"AMD Radeon RX 470 Series":   true, // gpu-radeon-rx-470-series
		"AMD Radeon RX Graphics":     true, // gpu-radeon-rx-graphics
		"AMD Radeon RX Series":       true, // gpu-radeon-rx-series
	}

	seen := map[string]bool{}
	for id, product := range marketing {
		if seen[strings.ToLower(product)] {
			continue
		}
		seen[strings.ToLower(product)] = true

		contested := index[id] != shorten(product)
		if contested && !want[product] {
			t.Errorf("%q is newly contested, and now resolves to %q", product, index[id])
		}
		if !contested && want[product] {
			t.Errorf("%q is no longer contested, and has renamed to %q", product, index[id])
		}
	}
}

// amdgpu.ids is not a function: these keys carry two names each, and libdrm
// returns the first. A refresh that inserts a row above one of these renames a
// live resource without contesting any name, so pin which row wins.
func TestVendoredIDsShadowedRowsArePinned(t *testing.T) {
	_, marketing, entries := loadVendored(t)

	want := map[ProductID]string{
		{DeviceID: "15D8", RevisionID: "91"}: "AMD Radeon Vega 3 Graphics",
		{DeviceID: "15D8", RevisionID: "92"}: "AMD Radeon Vega 3 Graphics",
		{DeviceID: "15D8", RevisionID: "DB"}: "AMD Radeon Vega 3 Graphics",
		{DeviceID: "6646", RevisionID: "80"}: "AMD Radeon R9 M385",
		{DeviceID: "6808", RevisionID: "00"}: "AMD FirePro W7000",
		{DeviceID: "743F", RevisionID: "C3"}: "AMD Radeon RX 6500",
		{DeviceID: "98E4", RevisionID: "EB"}: "AMD Radeon R3 Graphics",
	}

	got := map[ProductID]string{}
	for _, entry := range entries {
		if first := marketing[entry.id]; first != entry.marketing {
			got[entry.id] = first
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shadowed rows = %v, want %v", got, want)
	}
}

// Guards the vendored table itself: a refresh that renames a part, or adds one
// colliding with a datacenter part, changes resource names on live clusters.
func TestVendoredIDsNameDatacenterParts(t *testing.T) {
	index, _, _ := loadVendored(t)

	want := map[ProductID]string{
		{DeviceID: "740F", RevisionID: "02"}: "gpu-mi210",
		{DeviceID: "7408", RevisionID: "00"}: "gpu-mi250x",
		{DeviceID: "740C", RevisionID: "01"}: "gpu-mi250x_mi250",
		{DeviceID: "74A0", RevisionID: "00"}: "gpu-mi300a",
		{DeviceID: "74A1", RevisionID: "00"}: "gpu-mi300x",
		{DeviceID: "74B5", RevisionID: "00"}: "gpu-mi300x-vf",
		{DeviceID: "74A2", RevisionID: "00"}: "gpu-mi308x",
		{DeviceID: "74A5", RevisionID: "00"}: "gpu-mi325x",
		{DeviceID: "75A0", RevisionID: "00"}: "gpu-mi350x",
		{DeviceID: "75A3", RevisionID: "00"}: "gpu-mi355x",
	}
	for id, resource := range want {
		if got := index.Name(id.DeviceID, id.RevisionID); got != resource {
			t.Errorf("Name(%q, %q) = %q, want %q", id.DeviceID, id.RevisionID, got, resource)
		}
	}
}

// The feature's promise is that a resource name identifies a product. Two
// products sharing one name breaks it in the direction an operator cannot see:
// a ResourceQuota would silently cover both.
func TestVendoredIDsNameProductsUniquely(t *testing.T) {
	index, marketing, _ := loadVendored(t)

	claimants := map[string]map[string]bool{}
	for id, product := range marketing {
		resource, ok := index[id]
		if !ok {
			continue
		}
		if claimants[resource] == nil {
			claimants[resource] = map[string]bool{}
		}
		claimants[resource][strings.ToLower(product)] = true
	}

	for resource, products := range claimants {
		if len(products) <= 1 {
			continue
		}
		names := []string{}
		for product := range products {
			names = append(names, product)
		}
		sort.Strings(names)
		t.Errorf("%q is claimed by %d products: %q", resource, len(names), names)
	}
}

// "gpu" already means "this device is not in the table", so no derived name may
// equal it, and no row may silently fail to derive one.
func TestVendoredIDsNameEveryDeviceSomethingOtherThanTheDefault(t *testing.T) {
	index, marketing, _ := loadVendored(t)

	for id, product := range marketing {
		resource, ok := index[id]
		if !ok {
			t.Errorf("%s/%s %q derived no resource name", id.DeviceID, id.RevisionID, product)
			continue
		}
		if resource == DefaultResource {
			t.Errorf("%s/%s %q derived the default resource name", id.DeviceID, id.RevisionID, product)
		}
	}
}

// Composing with the longest partition style must stay inside the limit.
func TestVendoredIDsStayInsideTheLengthLimitWhenPartitioned(t *testing.T) {
	index, _, _ := loadVendored(t)

	for id, resource := range index {
		full := resource + "-" + SanitizeResourceName("cpx_nps4")
		if len(full) > maxResourceNameLen {
			t.Errorf("%s/%s partitioned is %d chars: %s", id.DeviceID, id.RevisionID, len(full), full)
		}
	}
}

// loadVendored returns the index, the product name each device resolves to,
// and the raw rows.
func loadVendored(t *testing.T) (ProductIndex, map[ProductID]string, []idEntry) {
	t.Helper()
	index, err := LoadProductIndex(vendoredIDs)
	if err != nil {
		t.Fatalf("LoadProductIndex(%s): %v", vendoredIDs, err)
	}
	entries, err := parseIDs(vendoredIDs)
	if err != nil {
		t.Fatalf("parseIDs(%s): %v", vendoredIDs, err)
	}
	marketing, _ := firstMatch(entries)
	return index, marketing, entries
}
