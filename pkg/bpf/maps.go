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
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"time"

	"github.com/AliyunContainerService/terway-qos/pkg/byteorder"
	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"github.com/cilium/ebpf"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// trafficDirection for config, MUST equal with bpf map index
	ingressIndex uint32 = 0
	egressIndex  uint32 = 1
)

var _ Interface = &Writer{}

type Writer struct {
	obj *qos_tcObjects
}

func (w *Writer) Close() {
	_ = w.obj.Close()
}

func NewMap() (*Writer, error) {
	w := &Writer{
		obj: getBpfObj(true),
	}

	return w, nil
}

func (w *Writer) GetGlobalConfig() (*types.GlobalConfig, *types.GlobalConfig, error) {
	ingress := &globalRateCfg{}
	egress := &globalRateCfg{}
	err := w.obj.TerwayGlobalCfg.Lookup(ingressIndex, ingress)
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return nil, nil, err
		}
	}
	err = w.obj.TerwayGlobalCfg.Lookup(egressIndex, egress)
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return nil, nil, err
		}
	}

	return &types.GlobalConfig{
			HwGuaranteed:   ingress.HwGuaranteed,
			HwBurstableBps: ingress.HwBurstable,
			L0MaxBps:       0,
			L0MinBps:       ingress.L0MinBps,
			L1MaxBps:       ingress.L1MaxBps,
			L1MinBps:       ingress.L1MinBps,
			L2MaxBps:       ingress.L2MaxBps,
			L2MinBps:       ingress.L2MinBps,
		}, &types.GlobalConfig{
			HwGuaranteed:   egress.HwGuaranteed,
			HwBurstableBps: egress.HwBurstable,
			L0MaxBps:       0,
			L0MinBps:       egress.L0MinBps,
			L1MaxBps:       egress.L1MaxBps,
			L1MinBps:       egress.L1MinBps,
			L2MaxBps:       egress.L2MaxBps,
			L2MinBps:       egress.L2MinBps,
		}, nil
}

func updateIfNotEqual(expect any, lookupo func() (any, error), update func() error) error {
	prev, err := lookupo()
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	if reflect.DeepEqual(prev, expect) {
		return nil
	}

	return update()
}

func (w *Writer) WriteGlobalConfig(ingress *types.GlobalConfig, egress *types.GlobalConfig) error {
	ingress.Default()
	if !ingress.Validate() {
		return fmt.Errorf("ingress config is not valid, %#v", *ingress)
	}
	egress.Default()
	if !egress.Validate() {
		return fmt.Errorf("egress config is not valid, %#v", *egress)
	}

	ingressCfg := &globalRateCfg{
		Interval:     uint64(500 * time.Millisecond),
		HwGuaranteed: ingress.HwGuaranteed,
		HwBurstable:  0,
		L0MinBps:     ingress.HwGuaranteed - ingress.L1MinBps - ingress.L2MinBps,
		L1MinBps:     ingress.L1MinBps,
		L1MaxBps:     ingress.L1MaxBps,
		L2MinBps:     ingress.L2MinBps,
		L2MaxBps:     ingress.L2MaxBps,
	}
	egressCfg := &globalRateCfg{
		Interval:     uint64(500 * time.Millisecond),
		HwGuaranteed: egress.HwGuaranteed,
		HwBurstable:  0,
		L0MinBps:     egress.HwGuaranteed - egress.L1MinBps - egress.L2MinBps,
		L1MinBps:     egress.L1MinBps,
		L1MaxBps:     egress.L1MaxBps,
		L2MinBps:     egress.L2MinBps,
		L2MaxBps:     egress.L2MaxBps,
	}

	err := updateIfNotEqual(ingressCfg, func() (any, error) {
		prev := &globalRateCfg{}
		err := w.obj.TerwayGlobalCfg.Lookup(ingressIndex, prev)
		return prev, err
	}, func() error {
		log.Info("write global config", "ingress", ingress.String())

		return w.obj.TerwayGlobalCfg.Put(ingressIndex, ingressCfg)
	})

	if err != nil {
		return err
	}
	err = updateIfNotEqual(egressCfg, func() (any, error) {
		prev := &globalRateCfg{}
		err := w.obj.TerwayGlobalCfg.Lookup(egressIndex, prev)
		return prev, err
	}, func() error {
		log.Info("write global config", "egress", egress.String())

		return w.obj.TerwayGlobalCfg.Put(egressIndex, egressCfg)
	})
	return err
}

func (w *Writer) WritePodInfo(config *types.PodConfig) error {
	if config.HostNetwork {
		return nil
	}
	info := &cgroupInfo{
		ClassID: config.CgroupInfo.ClassID,
		Pad1:    uint32(0),
		Inode:   config.CgroupInfo.Inode,
	}
	if config.IPv4.IsValid() {
		err := w.obj.PodMap.Put(ip2Addr(config.IPv4), info)
		if err != nil {
			return fmt.Errorf("error put pod_map map, %w", err)
		}
	}
	if config.IPv6.IsValid() {
		err := w.obj.PodMap.Put(ip2Addr(config.IPv6), info)
		if err != nil {
			return fmt.Errorf("error put pod_map map, %w", err)
		}
	}
	return w.WriteCgroupRate(&types.CgroupRate{
		Inode: config.CgroupInfo.Inode,
		RxBps: config.RxBps,
		TxBps: config.TxBps,
	})
}

func (w *Writer) DeletePodInfo(config *types.PodConfig) error {
	if config.HostNetwork {
		return nil
	}
	if config.IPv4.IsValid() {
		err := w.obj.PodMap.Delete(ip2Addr(config.IPv4))
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return fmt.Errorf("error put pod_map map by key %s, %w", config.IPv4, err)
			}
		}
	}
	if config.IPv6.IsValid() {
		err := w.obj.PodMap.Delete(ip2Addr(config.IPv6))
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return fmt.Errorf("error delete pod_map map by key %s, %w", config.IPv6, err)
			}
		}
	}
	return nil
}

func (w *Writer) ListPodInfo() map[netip.Addr]cgroupInfo {
	var result = map[netip.Addr]cgroupInfo{}
	var key addr
	var value cgroupInfo

	iter := w.obj.PodMap.Iterate()
	for iter.Next(&key, &value) {
		result[addr2ip(&key)] = value
	}
	return result
}

func (w *Writer) GetGlobalRateLimit() (*globalRateInfo, *globalRateInfo) {
	var ingress = &globalRateInfo{}
	var egress = &globalRateInfo{}
	_ = w.obj.GlobalRateMap.Lookup(ingressIndex, ingress)

	_ = w.obj.GlobalRateMap.Lookup(egressIndex, egress)
	return ingress, egress
}

func (w *Writer) GetCgroupRateInodes() sets.Set[uint64] {
	result := sets.New[uint64]()
	var key cgroupRateID
	var value rateInfo

	iter := w.obj.CgroupRateMap.Iterate()
	for iter.Next(&key, &value) {
		result[key.Inode] = struct{}{}
	}
	return result
}

func (w *Writer) DeleteCgroupRate(inode uint64) error {
	err := w.obj.CgroupRateMap.Delete(&cgroupRateID{
		Inode:     inode,
		Direction: egressIndex,
	})
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	err = w.obj.CgroupRateMap.Delete(&cgroupRateID{
		Inode:     inode,
		Direction: ingressIndex,
	})
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	return nil
}

func (w *Writer) WriteCgroupRate(r *types.CgroupRate) error {
	egressID := &cgroupRateID{
		Inode:     r.Inode,
		Direction: egressIndex,
	}
	ingressID := &cgroupRateID{
		Inode:     r.Inode,
		Direction: ingressIndex,
	}
	if r.RxBps == 0 {
		err := w.obj.CgroupRateMap.Delete(ingressID)
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return err
			}
		} else {
			log.Info("update rate", "ingress", r.RxBps)
		}
	} else {
		prev := &rateInfo{}
		err := w.obj.CgroupRateMap.Lookup(ingressID, prev)
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return err
			}
		}
		if prev.LimitBps == r.RxBps {
			return nil
		}
		log.Info("update rate", "rxBps", r.RxBps)

		err = w.obj.CgroupRateMap.Put(ingressID, &rateInfo{
			LimitBps:      r.RxBps,
			LastTimeStamp: 0,
		})
		if err != nil {
			return err
		}
	}
	if r.TxBps == 0 {
		err := w.obj.CgroupRateMap.Delete(egressID)
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return err
			}
		} else {
			log.Info("update rate", "txBps", r.TxBps)
		}
	} else {
		prev := &rateInfo{}
		err := w.obj.CgroupRateMap.Lookup(egressID, prev)
		if err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				return err
			}
		}
		if prev.LimitBps == r.TxBps {
			return nil
		}
		log.Info("update rate", "txBps", r.TxBps)

		err = w.obj.CgroupRateMap.Put(egressID, &rateInfo{
			LimitBps:      r.TxBps,
			LastTimeStamp: 0,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) GetNetStat() []netStat {
	var result []netStat
	ite := w.obj.TerwayNetStat.Iterate()
	var key uint32
	var stat netStat
	for ite.Next(&key, &stat) {
		result = append(result, stat)
	}
	return result
}

func ip2Addr(ip netip.Addr) *addr {
	slice := ip.As16()
	return &addr{
		D1: byteorder.HostToNetwork32(binary.BigEndian.Uint32(slice[:4])),
		D2: byteorder.HostToNetwork32(binary.BigEndian.Uint32(slice[4:8])),
		D3: byteorder.HostToNetwork32(binary.BigEndian.Uint32(slice[8:12])),
		D4: byteorder.HostToNetwork32(binary.BigEndian.Uint32(slice[12:])),
	}
}

func addr2ip(addr *addr) netip.Addr {
	slice := make([]byte, 0, 16)
	slice = binary.BigEndian.AppendUint32(slice, byteorder.NetworkToHost32(addr.D1))
	slice = binary.BigEndian.AppendUint32(slice, byteorder.NetworkToHost32(addr.D2))
	slice = binary.BigEndian.AppendUint32(slice, byteorder.NetworkToHost32(addr.D3))
	slice = binary.BigEndian.AppendUint32(slice, byteorder.NetworkToHost32(addr.D4))
	ip, _ := netip.AddrFromSlice(slice)
	return ip
}
