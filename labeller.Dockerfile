# Copyright 2022 Advanced Micro Devices, Inc.  All rights reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
ARG GOLANG_BASE_IMG=golang:1.26.7-alpine3.23
ARG ALPINE_BASE_IMG=alpine:3.23.5
FROM ${GOLANG_BASE_IMG}
RUN apk --no-cache add git pkgconfig build-base libdrm-dev wget
RUN mkdir -p /go/src/github.com/ROCm/k8s-device-plugin
ADD . /go/src/github.com/ROCm/k8s-device-plugin
WORKDIR /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller
RUN go install \
    -ldflags="-X main.gitDescribe=$(git -C /go/src/github.com/ROCm/k8s-device-plugin/ describe --always --long --dirty)"

FROM ${ALPINE_BASE_IMG}
LABEL \
    org.opencontainers.image.source="https://github.com/ROCm/k8s-device-plugin" \
    org.opencontainers.image.authors="Kenny Ho <Kenny.Ho@amd.com>" \
    org.opencontainers.image.vendor="Advanced Micro Devices, Inc." \
    org.opencontainers.image.licenses="Apache-2.0"
# See the note in Dockerfile: the published alpine image lags its own package
# repository, and 3.23.5 is the newest tag, so CVE-2026-14456 in libssl3 /
# libcrypto3 has no base bump that clears it.
RUN apk --no-cache upgrade
RUN apk --no-cache add ca-certificates libdrm
WORKDIR /root/
COPY --from=0 /go/bin/k8s-node-labeller .
COPY --from=0 /go/src/github.com/ROCm/k8s-device-plugin/cmd/k8s-node-labeller/amdgpu.ids /usr/share/libdrm/amdgpu.ids
CMD ["./k8s-node-labeller"]
