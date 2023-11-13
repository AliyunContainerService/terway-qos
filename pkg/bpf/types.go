/*
 * Copyright (c) 2023, Alibaba Group;
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bpf

import (
	"net/netip"

	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"k8s.io/apimachinery/pkg/util/sets"
)

type Interface interface {
	// WriteGlobalConfig write global limit
	WriteGlobalConfig(ingress *types.GlobalConfig, egress *types.GlobalConfig) error
	// WritePodInfo write class_id or rate limit for each pod
	WritePodInfo(config *types.PodConfig) error
	DeletePodInfo(config *types.PodConfig) error

	ListPodInfo() map[netip.Addr]cgroupInfo
	GetGlobalRateLimit() (*globalRateInfo, *globalRateInfo)

	WriteCgroupRate(config *types.CgroupRate) error
	DeleteCgroupRate(inode uint64) error
	GetCgroupRateInodes() sets.Set[uint64]
}

// rate for current rate and limit
type rateInfo struct {
	LimitBps      uint64 `ebpf:"bps"`
	LastTimeStamp uint64 `ebpf:"t_last"`
	Slot          uint64 `ebpf:"slot3"`
}

// addr for both ipv4 and ipv6
type addr struct {
	D1 uint32 `ebpf:"d1"`
	D2 uint32 `ebpf:"d2"`
	D3 uint32 `ebpf:"d3"`
	D4 uint32 `ebpf:"d4"`
}

// cgroupRateID
// store rx and tx in single map
type cgroupRateID struct {
	Inode     uint64 `ebpf:"inode"`
	Direction uint32 `ebpf:"direction"`
	Pad       uint32 `ebpf:"pad"`
}

type cgroupInfo struct {
	ClassID uint32 `ebpf:"class_id"`
	Pad1    uint32 `ebpf:"pad1"`
	Inode   uint64 `ebpf:"inode"`
}

type globalRateCfg struct {
	Interval     uint64 `ebpf:"interval"`
	HwGuaranteed uint64 `ebpf:"hw_min_bps"`
	HwBurstable  uint64 `ebpf:"hw_max_bps"`

	L0MinBps uint64 `ebpf:"l0_min_bps"`
	L0MaxBps uint64 `ebpf:"l0_max_bps"`
	L1MinBps uint64 `ebpf:"l1_min_bps"`
	L1MaxBps uint64 `ebpf:"l1_max_bps"`
	L2MinBps uint64 `ebpf:"l2_min_bps"`
	L2MaxBps uint64 `ebpf:"l2_max_bps"`
}

type globalRateInfo struct {
	LastTimestamp uint64 `ebpf:"t_last"`

	L0LastTimestamp uint64 `ebpf:"t_l0_last"`
	L0Bps           uint64 `ebpf:"l0_bps"`
	L0Slot          uint64 `ebpf:"l0_slot"`

	L1LastTimestamp uint64 `ebpf:"t_l1_last"`
	L1Bps           uint64 `ebpf:"l1_bps"`
	L1Slot          uint64 `ebpf:"l1_slot"`

	L2LastTimestamp uint64 `ebpf:"t_l2_last"`
	L2Bps           uint64 `ebpf:"l2_bps"`
	L2Slot          uint64 `ebpf:"l2_slot"`
}

type netStat struct {
	Index uint64 `ebpf:"index"`
	TS    uint64 `ebpf:"ts"`
	Val   uint64 `ebpf:"val"`
}
