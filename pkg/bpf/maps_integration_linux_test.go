package bpf

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"os"
	"testing"

	"github.com/AliyunContainerService/terway-qos/pkg/types"
	"github.com/cilium/ebpf"
)

// These tests create unpinned maps and load programs without attaching to links.
// Set TERWAY_QOS_BPF_TEST=1 on a Linux host with BPF privileges to run them.
func integrationWriter(t *testing.T) *Writer {
	t.Helper()
	if os.Getenv("TERWAY_QOS_BPF_TEST") != "1" {
		t.Skip("set TERWAY_QOS_BPF_TEST=1 for kernel BPF tests")
	}
	spec, err := loadQos_tc()
	if path := os.Getenv("TERWAY_QOS_BPF_OBJECT"); path != "" {
		spec, err = ebpf.LoadCollectionSpec(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range spec.Maps {
		m.Pinning = ebpf.PinNone
	}
	obj := &qos_tcObjects{}
	if err := spec.LoadAndAssign(obj, nil); err != nil {
		t.Fatal(err)
	}
	w := &Writer{obj: obj}
	t.Cleanup(w.Close)
	return w
}

func TestKernelCgroupRateUpdates(t *testing.T) {
	w := integrationWriter(t)
	r := &types.CgroupRate{Inode: 1, RxBps: 1000000, TxBps: 2000000}
	if err := w.WriteCgroupRate(r); err != nil {
		t.Fatal(err)
	}
	id := &cgroupRateID{Inode: 1, Direction: ingressIndex}
	// Unchanged limits must preserve the running token bucket.
	if err := w.obj.CgroupRateMap.Put(id, &rateInfo{LimitBps: r.RxBps, LastTimeStamp: 123, Slot: 456}); err != nil {
		t.Fatal(err)
	}
	r.TxBps = 3000000
	if err := w.WriteCgroupRate(r); err != nil {
		t.Fatal(err)
	}
	var rx, tx rateInfo
	if err := w.obj.CgroupRateMap.Lookup(id, &rx); err != nil {
		t.Fatal(err)
	}
	if rx.LastTimeStamp != 123 || rx.Slot != 456 {
		t.Fatal("unchanged ingress state reset")
	}
	id.Direction = egressIndex
	if err := w.obj.CgroupRateMap.Lookup(id, &tx); err != nil {
		t.Fatal(err)
	}
	if tx.LimitBps != 3000000 {
		t.Fatalf("egress = %d, want 3000000", tx.LimitBps)
	}
	r.TxBps = 0
	if err := w.WriteCgroupRate(r); err != nil {
		t.Fatal(err)
	}
	if err := w.obj.CgroupRateMap.Lookup(id, &tx); !errors.Is(err, ebpf.ErrKeyNotExist) {
		t.Fatalf("egress not removed: %v", err)
	}
	r.RxBps, r.TxBps = 4000000, 5000000
	if err := w.WriteCgroupRate(r); err != nil {
		t.Fatal(err)
	}
	id.Direction = ingressIndex
	if err := w.obj.CgroupRateMap.Lookup(id, &rx); err != nil {
		t.Fatal(err)
	}
	if rx.LimitBps != 4000000 || rx.LastTimeStamp != 0 || rx.Slot != 0 {
		t.Fatalf("changed ingress state = %+v", rx)
	}
}

func TestKernelPodDeletion(t *testing.T) {
	w := integrationWriter(t)
	rate := uint64(2000000)
	p := &types.PodConfig{IPv4: netip.MustParseAddr("10.0.0.2"), IPv6: netip.MustParseAddr("2001:db8::2"),
		CgroupInfo: &types.CgroupInfo{Inode: 42}, RxBps: &rate, TxBps: &rate}
	if err := w.WritePodInfo(p); err != nil {
		t.Fatal(err)
	}
	if len(w.ListPodInfo()) != 2 || len(w.ListCgroupRate()) != 2 {
		t.Fatal("missing pod maps")
	}
	if err := w.DeletePodInfo(p); err != nil {
		t.Fatal(err)
	}
	if len(w.ListPodInfo()) != 0 || len(w.ListCgroupRate()) != 0 {
		t.Fatal("deleted pod leaked map entries")
	}
	if err := w.DeletePodInfo(p); err != nil {
		t.Fatal(err)
	}
}

func TestKernelPruneOrphanedLimits(t *testing.T) {
	w := integrationWriter(t)
	for _, inode := range []uint64{1, 2, 3} {
		if err := w.WriteCgroupRate(&types.CgroupRate{Inode: inode, RxBps: 1000000, TxBps: 2000000}); err != nil {
			t.Fatal(err)
		}
	}
	// Keep both annotation- and file-derived limits, remove the restart orphan.
	keep := map[uint64]struct{}{1: {}, 2: {}}
	if err := w.PruneCgroupRates(keep); err != nil {
		t.Fatal(err)
	}
	remaining := w.ListCgroupRate()
	if len(remaining) != 4 {
		t.Fatalf("remaining limits = %v", remaining)
	}
	for key := range remaining {
		if key.Inode == 3 {
			t.Fatal("orphaned direction survived")
		}
	}
	if err := w.PruneCgroupRates(nil); err != nil {
		t.Fatal(err)
	}
	if len(w.ListCgroupRate()) != 0 {
		t.Fatal("empty snapshot did not remove orphaned limits")
	}
}

func TestKernelIngressPackets(t *testing.T) {
	w := integrationWriter(t)
	if err := w.obj.QosProgMap.Put(uint32(0), uint32(w.obj.QosCgroup.FD())); err != nil {
		t.Fatal(err)
	}
	rate := uint64(1000000)
	p := &types.PodConfig{IPv4: netip.MustParseAddr("10.0.0.2"),
		CgroupInfo: &types.CgroupInfo{Inode: 42}, RxBps: &rate}
	if err := w.WritePodInfo(p); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 1500)
	binary.BigEndian.PutUint16(packet[12:14], 0x0800)
	packet[14] = 0x45
	binary.BigEndian.PutUint16(packet[16:18], uint16(len(packet)-14))
	packet[22], packet[23] = 64, 17
	copy(packet[26:30], []byte{10, 0, 0, 1})
	copy(packet[30:34], []byte{10, 0, 0, 2})
	// A burst exceeding the bucket capacity must eventually be dropped.
	ret, _, err := w.obj.QosProgIngress.Benchmark(packet, 10000, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ret != 2 {
		t.Fatalf("limited burst returned %d, want TC_ACT_SHOT", ret)
	}
	rate = 0
	if err := w.WritePodInfo(p); err != nil {
		t.Fatal(err)
	}
	ret, _, err = w.obj.QosProgIngress.Test(packet)
	if err != nil {
		t.Fatal(err)
	}
	if ret != 3 {
		t.Fatalf("unlimited packet returned %d, want TC_ACT_PIPE", ret)
	}
}
