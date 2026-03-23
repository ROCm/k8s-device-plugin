# Makefile for building and packaging ROCm K8s Device Plugin Docker images

# Use bash with pipefail to catch errors in pipe chains
SHELL := /bin/bash
.SHELLFLAGS := -o pipefail -c

# Image repository and version
IMAGE_REPO := docker.io/rocm/k8s-device-plugin
IMAGE_VERSION ?= latest

# Image tags based on the repository's tagging scheme
# Alpine device plugin: latest / <version>
# Alpine labeller: labeller-latest / labeller-<version>
# UBI device plugin: rhubi-latest / rhubi-<version>
# UBI labeller: labeller-rhubi-latest / labeller-rhubi-<version>

DEVICE_PLUGIN_TAG := $(IMAGE_VERSION)
LABELLER_TAG := labeller-$(IMAGE_VERSION)
UBI_DEVICE_PLUGIN_TAG := rhubi-$(IMAGE_VERSION)
UBI_LABELLER_TAG := labeller-rhubi-$(IMAGE_VERSION)

# Output directory for tar.gz files
OUTPUT_DIR ?= ./dist
TAR_DIR ?= $(OUTPUT_DIR)/tarballs

.PHONY: all build-all save-all clean help
.PHONY: build-device-plugin build-labeller build-ubi-device-plugin build-ubi-labeller
.PHONY: save-device-plugin save-labeller save-ubi-device-plugin save-ubi-labeller

# Default target
all: build-all save-all

# Build all images
build-all: build-device-plugin build-labeller build-ubi-device-plugin build-ubi-labeller
	@echo "All images built successfully"

# Save all images to tar.gz
save-all: save-device-plugin save-labeller save-ubi-device-plugin save-ubi-labeller
	@echo "All images saved to $(TAR_DIR)/"

# Build Alpine-based device plugin image
build-device-plugin:
	@echo "Building Alpine-based device plugin image..."
	docker build -t $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG) -f Dockerfile .
	@echo "Built: $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG)"

# Build Alpine-based node labeller image
build-labeller:
	@echo "Building Alpine-based node labeller image..."
	docker build -t $(IMAGE_REPO):$(LABELLER_TAG) -f labeller.Dockerfile .
	@echo "Built: $(IMAGE_REPO):$(LABELLER_TAG)"

# Build UBI-based device plugin image
build-ubi-device-plugin:
	@echo "Building UBI-based device plugin image..."
	docker build -t $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG) -f ubi-dp.Dockerfile .
	@echo "Built: $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG)"

# Build UBI-based node labeller image
build-ubi-labeller:
	@echo "Building UBI-based node labeller image..."
	docker build -t $(IMAGE_REPO):$(UBI_LABELLER_TAG) -f ubi-labeller.Dockerfile .
	@echo "Built: $(IMAGE_REPO):$(UBI_LABELLER_TAG)"

# Save Alpine device plugin image to tar.gz
save-device-plugin: build-device-plugin
	@echo "Saving $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG) to tar.gz..."
	@mkdir -p $(TAR_DIR)
	docker save $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG) | gzip > $(TAR_DIR)/k8s-device-plugin-$(DEVICE_PLUGIN_TAG).tar.gz
	@echo "Saved to: $(TAR_DIR)/k8s-device-plugin-$(DEVICE_PLUGIN_TAG).tar.gz"

# Save Alpine node labeller image to tar.gz
save-labeller: build-labeller
	@echo "Saving $(IMAGE_REPO):$(LABELLER_TAG) to tar.gz..."
	@mkdir -p $(TAR_DIR)
	docker save $(IMAGE_REPO):$(LABELLER_TAG) | gzip > $(TAR_DIR)/k8s-node-labeller-$(IMAGE_VERSION).tar.gz
	@echo "Saved to: $(TAR_DIR)/k8s-node-labeller-$(IMAGE_VERSION).tar.gz"

# Save UBI device plugin image to tar.gz
save-ubi-device-plugin: build-ubi-device-plugin
	@echo "Saving $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG) to tar.gz..."
	@mkdir -p $(TAR_DIR)
	docker save $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG) | gzip > $(TAR_DIR)/k8s-device-plugin-$(UBI_DEVICE_PLUGIN_TAG).tar.gz
	@echo "Saved to: $(TAR_DIR)/k8s-device-plugin-$(UBI_DEVICE_PLUGIN_TAG).tar.gz"

# Save UBI node labeller image to tar.gz
save-ubi-labeller: build-ubi-labeller
	@echo "Saving $(IMAGE_REPO):$(UBI_LABELLER_TAG) to tar.gz..."
	@mkdir -p $(TAR_DIR)
	docker save $(IMAGE_REPO):$(UBI_LABELLER_TAG) | gzip > $(TAR_DIR)/k8s-node-labeller-rhubi-$(IMAGE_VERSION).tar.gz
	@echo "Saved to: $(TAR_DIR)/k8s-node-labeller-rhubi-$(IMAGE_VERSION).tar.gz"

# Clean up built images and tar files
clean:
	@echo "Cleaning up Docker images and tar files..."
	-docker rmi $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG) 2>/dev/null
	-docker rmi $(IMAGE_REPO):$(LABELLER_TAG) 2>/dev/null
	-docker rmi $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG) 2>/dev/null
	-docker rmi $(IMAGE_REPO):$(UBI_LABELLER_TAG) 2>/dev/null
	-rm -rf $(OUTPUT_DIR)
	@echo "Cleanup complete"

# Help target
help:
	@echo "ROCm K8s Device Plugin - Makefile targets"
	@echo ""
	@echo "Usage: make [target] [IMAGE_VERSION=<version>]"
	@echo ""
	@echo "Available targets:"
	@echo "  all                    - Build and save all images (default)"
	@echo "  build-all              - Build all Docker images"
	@echo "  save-all               - Save all Docker images to tar.gz"
	@echo ""
	@echo "Individual build targets:"
	@echo "  build-device-plugin    - Build Alpine device plugin (tag: $(DEVICE_PLUGIN_TAG))"
	@echo "  build-labeller         - Build Alpine node labeller (tag: $(LABELLER_TAG))"
	@echo "  build-ubi-device-plugin - Build UBI device plugin (tag: $(UBI_DEVICE_PLUGIN_TAG))"
	@echo "  build-ubi-labeller     - Build UBI node labeller (tag: $(UBI_LABELLER_TAG))"
	@echo ""
	@echo "Individual save targets:"
	@echo "  save-device-plugin     - Build and save Alpine device plugin to tar.gz"
	@echo "  save-labeller          - Build and save Alpine node labeller to tar.gz"
	@echo "  save-ubi-device-plugin - Build and save UBI device plugin to tar.gz"
	@echo "  save-ubi-labeller      - Build and save UBI node labeller to tar.gz"
	@echo ""
	@echo "Other targets:"
	@echo "  clean                  - Remove built images and tar.gz files"
	@echo "  help                   - Show this help message"
	@echo ""
	@echo "Variables:"
	@echo "  IMAGE_VERSION          - Version suffix for tags (default: latest)"
	@echo "  OUTPUT_DIR             - Base output directory (default: ./dist)"
	@echo "  TAR_DIR                - Directory for tarball output (default: ./dist/tarballs)"
	@echo ""
	@echo "Tag mapping (using IMAGE_VERSION=$(IMAGE_VERSION)):"
	@echo "  Alpine device plugin:  $(IMAGE_REPO):$(DEVICE_PLUGIN_TAG)"
	@echo "  Alpine labeller:       $(IMAGE_REPO):$(LABELLER_TAG)"
	@echo "  UBI device plugin:     $(IMAGE_REPO):$(UBI_DEVICE_PLUGIN_TAG)"
	@echo "  UBI labeller:          $(IMAGE_REPO):$(UBI_LABELLER_TAG)"
	@echo ""
	@echo "Examples:"
	@echo "  make all                                # Builds all with 'latest' version"
	@echo "  make build-all IMAGE_VERSION=1.31.0.9   # Build all with version 1.31.0.9"
	@echo "  make save-device-plugin                 # Build and save device plugin only"
	@echo "  make clean                              # Clean up images and tar files"
