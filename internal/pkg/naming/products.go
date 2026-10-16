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
	"bufio"
	"fmt"
	"os"
	"strings"
)

// IDsPath is where the image installs amdgpu.ids.
const IDsPath = "/usr/share/libdrm/amdgpu.ids"

// vendorWords say nothing a resource under amd.com/ does not already say.
var vendorWords = map[string]bool{"amd": true, "ati": true}

// filler adds the category words. "pro" and "rx" are deliberately absent: they
// separate Radeon Pro VII from Radeon VII.
var filler = map[string]bool{
	"amd": true, "ati": true, "instinct": true,
	"radeon": true, "graphics": true, "series": true,
}

// ProductID is the key amdgpu.ids is written against: a PCI device id and
// revision id, spelled in upper-case hex without the 0x, as the table spells
// them. Reading these from sysfs is what lets the plugin name a GPU without
// opening its DRM node.
type ProductID struct {
	DeviceID   string
	RevisionID string
}

// ProductIndex maps a device to the resource name it is advertised under.
type ProductIndex map[ProductID]string

// Name returns "" if the table has no entry for the device.
func (p ProductIndex) Name(deviceID, revisionID string) string {
	return p[ProductID{
		DeviceID:   normalizeID(deviceID),
		RevisionID: normalizeID(revisionID),
	}]
}

// normalizeID puts a sysfs "0x74a1" and a table "74A1" into one spelling.
func normalizeID(id string) string {
	id = strings.TrimSpace(id)
	return strings.ToUpper(strings.TrimPrefix(strings.ToLower(id), "0x"))
}

// dropWords builds a resource name from the words not in drop, or "" if that
// leaves nothing.
//
// A "/" is a disjunction: one device id libdrm cannot resolve to a single SKU.
// It does not mean the same thing on every row -- "MI250X / MI250" is two SKUs
// behind one id, "R9 200 / HD 7900 Series" is one board sold under two names,
// and "RX Vega (mobile / OEM)" is not a product separator at all -- so the
// alternatives are joined with "_" and left for a reader who knows the
// hardware, rather than resolved here into a claim the table does not make.
func dropWords(marketing string, drop map[string]bool) string {
	alternatives := []string{}
	for _, alternative := range strings.Split(marketing, "/") {
		kept := []string{}
		for _, tok := range strings.Fields(strings.ToLower(alternative)) {
			if clean := SanitizeResourceName(tok); clean != "" && !drop[clean] {
				kept = append(kept, clean)
			}
		}
		if len(kept) > 0 {
			alternatives = append(alternatives, strings.Join(kept, "-"))
		}
	}
	if len(alternatives) == 0 {
		return ""
	}
	return truncate(DefaultResource + "-" + strings.Join(alternatives, "_"))
}

// shorten leaves "AMD Instinct MI300X" as "gpu-mi300x". A name that is nothing
// but category words -- "AMD Radeon Series" -- keeps them, because reducing it
// to a bare "gpu" would make a real product indistinguishable from one the
// table does not list.
func shorten(marketing string) string {
	if short := dropWords(marketing, filler); short != "" {
		return short
	}
	return qualify(marketing)
}

// qualify keeps the category words that shorten drops, for names that need them
// to stay distinct: "gpu-radeon-540" against "gpu-radeon-540-series".
func qualify(marketing string) string {
	return dropWords(marketing, vendorWords)
}

// LoadProductIndex maps every device in an amdgpu.ids table to a resource name.
// Deriving the names from the whole table rather than from the GPUs present is
// what makes a node answer the same way whatever hardware it holds.
func LoadProductIndex(path string) (ProductIndex, error) {
	entries, err := parseIDs(path)
	if err != nil {
		return nil, err
	}

	marketing, products := firstMatch(entries)
	canonical, folded := foldCase(products)
	resources := resolve(folded)

	index := ProductIndex{}
	for id, product := range marketing {
		// A name that derives to bare "gpu", or to nothing at all, cannot be
		// told apart from a device the table does not list, so leave it out
		// and let it fall back explicitly.
		if name := resources[canonical[strings.ToLower(product)]]; name != "" && name != DefaultResource {
			index[id] = name
		}
	}
	return index, nil
}

// foldCase treats two names differing only in case as one product. It returns
// the spelling each folds to, and the distinct names in table order.
func foldCase(products []string) (map[string]string, []string) {
	canonical := map[string]string{}
	folded := []string{}
	for _, product := range products {
		if _, seen := canonical[strings.ToLower(product)]; !seen {
			canonical[strings.ToLower(product)] = product
			folded = append(folded, product)
		}
	}
	return canonical, folded
}

// resolve gives each product its resource name. A short name two products claim
// sends both back to the form carrying the category words, so that one resource
// never covers two products.
func resolve(products []string) map[string]string {
	claims := map[string][]string{}
	for _, product := range products {
		if short := shorten(product); short != "" {
			claims[short] = append(claims[short], product)
		}
	}

	resources := map[string]string{}
	for short, claimants := range claims {
		if len(claimants) == 1 {
			resources[claimants[0]] = short
			continue
		}
		for _, product := range claimants {
			resources[product] = qualify(product)
		}
	}
	return resources
}

// firstMatch resolves each device to one product name, and returns those names
// in table order.
//
// The table is not a function: 7 keys in the vendored copy carry two different
// names. libdrm stops at the first row that matches a device, so match that, or
// a GPU would be named one thing here and another by the labeller's
// gpu.product-name label.
func firstMatch(entries []idEntry) (map[ProductID]string, []string) {
	marketing := map[ProductID]string{}
	products := []string{}
	for _, entry := range entries {
		if _, claimed := marketing[entry.id]; !claimed {
			marketing[entry.id] = entry.marketing
			products = append(products, entry.marketing)
		}
	}
	return marketing, products
}

// idEntry is one row of an amdgpu.ids table.
type idEntry struct {
	id        ProductID
	marketing string
}

// parseIDs returns the rows of an amdgpu.ids table, whose lines are
// "device_id, revision_id, product_name" after a header and a version.
func parseIDs(path string) ([]idEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read amdgpu.ids: %w", err)
	}
	defer f.Close()

	entries := []idEntry{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.SplitN(line, ",", 3)
		if len(fields) != 3 {
			continue
		}
		entry := idEntry{
			id: ProductID{
				DeviceID:   normalizeID(fields[0]),
				RevisionID: normalizeID(fields[1]),
			},
			marketing: strings.TrimSpace(fields[2]),
		}
		if entry.id.DeviceID == "" || entry.id.RevisionID == "" || entry.marketing == "" {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read amdgpu.ids: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no product names in %s", path)
	}
	return entries, nil
}
