#!/bin/bash

LIBBPF_VERSION=0.6.1
CILIUM_VERSION=1.13.1

# The headers we want
LIBBPF_HEADERS=(
    libbpf-"$LIBBPF_VERSION"/LICENSE.BSD-2-Clause
    libbpf-"$LIBBPF_VERSION"/src/bpf_endian.h
    libbpf-"$LIBBPF_VERSION"/src/bpf_helper_defs.h
    libbpf-"$LIBBPF_VERSION"/src/bpf_helpers.h
    libbpf-"$LIBBPF_VERSION"/src/bpf_tracing.h
)

LINUX_HEADERS=(
    cilium-"$CILIUM_VERSION"/bpf/include/linux/in.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/in6.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/ip.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/ipv6.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/if_ether.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/bpf.h
    cilium-"$CILIUM_VERSION"/bpf/include/linux/bpf_common.h
    cilium-"$CILIUM_VERSION"/bpf/include/bpf/types_mapper.h
)

TMP_DIR=$(mktemp -d)

PROJECT_HEADERS_DIR=$(dirname ${BASH_SOURCE[0]})
LIBBPF_TAR=libbpf-v${LIBBPF_VERSION}.tar.gz
CILIUM_TAR=cilium-v${CILIUM_VERSION}.tar.gz

curl -sL "https://github.com/libbpf/libbpf/archive/refs/tags/v${LIBBPF_VERSION}.tar.gz" -o "${TMP_DIR}/${LIBBPF_TAR}"
tar -xvf "${TMP_DIR}/${LIBBPF_TAR}" -C "${TMP_DIR}" 2> /dev/null

for file in "${LIBBPF_HEADERS[@]}"; do
  cp "${TMP_DIR}/$file" "$PROJECT_HEADERS_DIR/"
done;

curl -sL "https://github.com/cilium/cilium/archive/refs/tags/v${CILIUM_VERSION}.tar.gz" -o "${TMP_DIR}/${CILIUM_TAR}"
tar -xvf "${TMP_DIR}/${CILIUM_TAR}" -C "${TMP_DIR}" 2> /dev/null

for file in "${LINUX_HEADERS[@]}"; do
  cp "${TMP_DIR}/$file" "$PROJECT_HEADERS_DIR/linux/"
done;

rm -rf "$TMP_DIR"
