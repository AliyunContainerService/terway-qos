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

	"k8s.io/klog/v2"

	"github.com/AliyunContainerService/terway-qos/pkg/byteorder"
	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"github.com/cilium/ebpf"
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

func updateIfNotEqual(expect any, idx uint32, lookupo func(idx uint32) (any, error), update func(idx uint32, rateCfg any) error) error {
	prev, err := lookupo(idx)
	if err != nil {
		if !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}
	if reflect.DeepEqual(prev, expect) {
		return nil
	}

	return update(idx, expect)
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

	lookRateFunc := func(idx uint32) (any, error) {
		prev := &globalRateCfg{}
		err := w.obj.TerwayGlobalCfg.Lookup(idx, prev)
		return prev, err
	}

	updateRateFunc := func(idx uint32, rateCfg any) error {
		idxtostr := map[uint32]string{
			ingressIndex: "ingress",
			egressIndex:  "egress",
		}
		log.Info("write global config", idxtostr[idx], egress.String())
		return w.obj.TerwayGlobalCfg.Put(idx, rateCfg)
	}

	if err := updateIfNotEqual(ingressCfg, ingressIndex, lookRateFunc, updateRateFunc); err != nil {
		return err
	}
	return updateIfNotEqual(egressCfg, egressIndex, lookRateFunc, updateRateFunc)
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

	klog.Infof("write pod info, %v", config)
	rx := uint64(0)
	tx := uint64(0)
	if config.RxBps != nil {
		rx = *config.RxBps
	}
	if config.TxBps != nil {
		tx = *config.TxBps
	}

	return w.WriteCgroupRate(&types.CgroupRate{
		Inode: config.CgroupInfo.Inode,
		RxBps: rx,
		TxBps: tx,
	})
}

func (w *Writer) DeletePodInfo(config *types.PodConfig) error {
	if config.HostNetwork {
		return nil
	}

	ips := []netip.Addr{config.IPv4, config.IPv6}
	for _, ip := range ips {
		if !ip.IsValid() {
			continue
		}
		if err := w.obj.PodMap.Delete(ip2Addr(ip)); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("error put pod_map map by key %s, %w", ip, err)
		}
	}

	return w.DeleteCgroupRate(config.CgroupInfo.Inode)
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

func (w *Writer) ListCgroupRate() map[cgroupRateID]rateInfo {
	result := make(map[cgroupRateID]rateInfo)
	var key cgroupRateID
	var value rateInfo

	iter := w.obj.CgroupRateMap.Iterate()
	for iter.Next(&key, &value) {
		result[key] = value
	}
	return result
}

func (w *Writer) DeleteCgroupRate(inode uint64) error {
	direction := []uint32{egressIndex, ingressIndex}
	for _, cur := range direction {
		obj := &cgroupRateID{
			Inode:     inode,
			Direction: cur,
		}
		if err := w.obj.CgroupRateMap.Delete(obj); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
	}

	return nil
}

// PruneCgroupRates first reads the complete map, then removes both directions
// for orphaned inodes. Iteration failure must never trigger partial cleanup.
func (w *Writer) PruneCgroupRates(keep map[uint64]struct{}) error {
	stale := make(map[uint64]struct{})
	var key cgroupRateID
	var value rateInfo
	iter := w.obj.CgroupRateMap.Iterate()
	for iter.Next(&key, &value) {
		if _, ok := keep[key.Inode]; !ok {
			stale[key.Inode] = struct{}{}
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	for inode := range stale {
		if err := w.DeleteCgroupRate(inode); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) WriteCgroupRate(r *types.CgroupRate) error {
	if err := w.writeDirectionRate(r.Inode, ingressIndex, r.RxBps); err != nil {
		return err
	}
	return w.writeDirectionRate(r.Inode, egressIndex, r.TxBps)
}

func (w *Writer) writeDirectionRate(inode uint64, direction uint32, limit uint64) error {
	id := &cgroupRateID{Inode: inode, Direction: direction}
	if limit == 0 {
		if err := w.obj.CgroupRateMap.Delete(id); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return err
		}
		return nil
	}
	prev := &rateInfo{}
	if err := w.obj.CgroupRateMap.Lookup(id, prev); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return err
	}
	if prev.LimitBps == limit {
		return nil
	}
	return w.obj.CgroupRateMap.Put(id, &rateInfo{LimitBps: limit})
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
