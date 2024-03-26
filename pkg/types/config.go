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

package types

import (
	"fmt"
	"net/netip"
)

type SyncPod interface {
	DeletePod(id string) error
	UpdatePod(config *PodConfig) error
}

// PodConfig contain pod related resource
type PodConfig struct {
	PodID  string
	PodUID string

	IPv4 netip.Addr
	IPv6 netip.Addr

	HostNetwork bool
	Prio        *uint32

	CgroupInfo *CgroupInfo

	RxBps *uint64
	TxBps *uint64
}

type CgroupInfo struct {
	Path    string
	ClassID uint32
	Inode   uint64
}

type CgroupRate struct {
	Inode uint64

	RxBps uint64
	TxBps uint64
}

type GlobalConfig struct {
	HwGuaranteed   uint64
	HwBurstableBps uint64

	L0MaxBps uint64
	L0MinBps uint64

	L1MaxBps uint64
	L1MinBps uint64

	L2MaxBps uint64
	L2MinBps uint64
}

func (c *GlobalConfig) Default() {
	if c.HwGuaranteed != 0 && c.HwBurstableBps == 0 {
		c.HwBurstableBps = c.HwGuaranteed
	}
	if c.L0MaxBps == 0 {
		c.L0MaxBps = c.HwGuaranteed
	}
	if c.L0MinBps == 0 {
		c.L0MinBps = c.HwGuaranteed - c.L1MinBps - c.L2MinBps
	}
}

func (c *GlobalConfig) Validate() bool {
	if c.HwBurstableBps == 0 && c.L0MaxBps == 0 && c.L0MinBps == 0 && c.L1MaxBps == 0 && c.L1MinBps == 0 && c.L2MaxBps == 0 && c.L2MinBps == 0 {
		return true
	}

	if c.HwGuaranteed > c.HwBurstableBps ||
		c.HwGuaranteed < c.L1MaxBps ||
		c.HwGuaranteed < c.L2MaxBps ||
		c.L1MinBps > c.L1MaxBps ||
		c.L2MinBps > c.L2MaxBps ||
		c.HwGuaranteed < c.L2MaxBps+c.L1MaxBps {
		return false
	}

	return true
}

func (c *GlobalConfig) String() string {
	return fmt.Sprintf("hw %d online-min %d online-max %d offline-l1-min %d offline-l1-max %d offline-l2-min %d offline-l2-max %d",
		c.HwGuaranteed, c.L0MinBps, c.L0MaxBps, c.L1MinBps, c.L1MaxBps, c.L2MinBps, c.L2MaxBps)
}
