package config

import (
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"
)

type fakeWriter struct {
	bpf.Interface
	written    map[string]*types.PodConfig
	deleted    []uint64
	err        error
	rates      map[uint64]struct{}
	pruneCalls int
	pruneErr   error
}

func (w *fakeWriter) PruneCgroupRates(keep map[uint64]struct{}) error {
	w.pruneCalls++
	if w.pruneErr != nil {
		return w.pruneErr
	}
	for inode := range w.rates {
		if _, ok := keep[inode]; !ok {
			delete(w.rates, inode)
		}
	}
	return nil
}

func (w *fakeWriter) WritePodInfo(p *types.PodConfig) error {
	if w.err != nil {
		return w.err
	}
	w.written[p.PodID] = p
	return nil
}

func (w *fakeWriter) DeletePodInfo(p *types.PodConfig) error {
	if w.err != nil {
		return w.err
	}
	w.deleted = append(w.deleted, p.CgroupInfo.Inode)
	delete(w.written, p.PodID)
	return nil
}

type fakeCgroup struct {
	inode   uint64
	classID uint32
	groups  map[string]types.CgroupInfo
	listErr error
}

func (c *fakeCgroup) ListCgroups() (map[string]types.CgroupInfo, error) { return c.groups, c.listErr }

func (c *fakeCgroup) GetCgroupByPodUID(uid string) (*types.CgroupInfo, error) {
	c.inode++
	return &types.CgroupInfo{Path: "/cgroup/" + uid, Inode: c.inode}, nil
}
func (c *fakeCgroup) SetCgroupClassID(prio uint32, _ string) error { c.classID = prio; return nil }

func testSyncer(t *testing.T) (*Syncer, *fakeWriter) {
	t.Helper()
	w := &fakeWriter{written: make(map[string]*types.PodConfig)}
	s := NewSyncer(w)
	s.cgroup = &fakeCgroup{}
	dir := t.TempDir()
	s.perCgroupPath = filepath.Join(dir, "per_cgroup_bps_limit")
	s.podConfigPath = filepath.Join(dir, "pod.json")
	return s, w
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func checkRate(t *testing.T, p *types.PodConfig, rx, tx uint64) {
	t.Helper()
	var gotRx, gotTx uint64
	if p.RxBps != nil {
		gotRx = *p.RxBps
	}
	if p.TxBps != nil {
		gotTx = *p.TxBps
	}
	if gotRx != rx || gotTx != tx {
		t.Fatalf("rates = (%d,%d), want (%d,%d)", gotRx, gotTx, rx, tx)
	}
}

func TestSyncerConfigurationSources(t *testing.T) {
	s, w := testSyncer(t)
	writeConfig(t, s.perCgroupPath, "/cgroup/a rx_bps=100 tx_bps=200\n/cgroup/b rx_bps=300 tx_bps=400\n")
	writeConfig(t, s.podConfigPath, `{"a":{"cgroupDir":"/cgroup/a","prio":1,"qosConfig":{"ingressBandwidth":500,"egressBandwidth":600}}}`)
	// Configuration arriving before the Pod must be retained.
	if err := s.syncFilePods(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := s.UpdatePod(&types.PodConfig{PodID: id, PodUID: id}); err != nil {
			t.Fatal(err)
		}
	}
	checkRate(t, w.written["a"], 500, 600)
	checkRate(t, w.written["b"], 300, 400)
	if w.written["a"].CgroupInfo.ClassID != 1 {
		t.Fatal("file priority not applied")
	}
	// A new annotation overrides only its direction and remains effective on file sync.
	annotation := uint64(700)
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", TxBps: &annotation}); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 500, 700)
	annotation = 800
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", TxBps: &annotation}); err != nil {
		t.Fatal(err)
	}
	if err := s.syncPodConfig(); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 500, 800)
	// Removing an annotation restores the file value.
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a"}); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 500, 600)
	if err := os.Remove(s.podConfigPath); err != nil {
		t.Fatal(err)
	}
	if err := s.syncPodConfig(); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 100, 200)
	checkRate(t, w.written["b"], 300, 400)
	if w.written["a"].CgroupInfo.ClassID != 0 {
		t.Fatal("removed file priority not restored")
	}
	if err := os.Remove(s.perCgroupPath); err != nil {
		t.Fatal(err)
	}
	if err := s.syncCgroupRate(); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 0, 0)
	checkRate(t, w.written["b"], 0, 0)
}

func TestMissingFilesPreserveAnnotations(t *testing.T) {
	s, w := testSyncer(t)
	rate := uint64(100)
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", RxBps: &rate}); err != nil {
		t.Fatal(err)
	}
	if err := s.syncCgroupRate(); err != nil {
		t.Fatal(err)
	}
	if err := s.syncPodConfig(); err != nil {
		t.Fatal(err)
	}
	checkRate(t, w.written["a"], 100, 0)
}

func TestInvalidFilePreservesSnapshot(t *testing.T) {
	s, w := testSyncer(t)
	writeConfig(t, s.perCgroupPath, "/cgroup/a rx_bps=100 tx_bps=200")
	if err := s.syncFilePods(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a"}); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{`{`, `{"a":null}`, `{"a":{"cgroupDir":"relative"}}`, `{"a":{"cgroupDir":"/cgroup/a","prio":9}}`, `{"a":{"cgroupDir":"/cgroup/a"},"b":{"cgroupDir":"/cgroup/a"}}`} {
		writeConfig(t, s.podConfigPath, content)
		if err := s.syncFilePods(); err == nil {
			t.Fatalf("accepted invalid config %s", content)
		}
		if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a"}); err != nil {
			t.Fatal(err)
		}
		checkRate(t, w.written["a"], 100, 200)
	}
}

func TestLegacyFileValidation(t *testing.T) {
	s, w := testSyncer(t)
	writeConfig(t, s.perCgroupPath, "# bandwidth in bytes/s\n  /cgroup/a rx_bps = 100 tx_bps 200\n")
	if err := s.syncFilePods(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a"}); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"/cgroup/a rx_bps=-1", "/cgroup/a rx_bps=bad", "/cgroup/a tx_bps=18446744073709551616",
		"/cgroup/a tx_bps", "/cgroup/a tx_bps=1 tx_bps=2", "/cgroup/a unknown=1",
	} {
		writeConfig(t, s.perCgroupPath, line)
		if err := s.syncFilePods(); err == nil {
			t.Fatalf("accepted invalid bandwidth: %s", line)
		}
		if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a"}); err != nil {
			t.Fatal(err)
		}
		checkRate(t, w.written["a"], 100, 200)
	}
}

func TestIPChangeRemovesOldMapping(t *testing.T) {
	s, w := testSyncer(t)
	p := &types.PodConfig{PodID: "a", PodUID: "a", IPv4: netip.MustParseAddr("10.0.0.1")}
	if err := s.UpdatePod(p); err != nil {
		t.Fatal(err)
	}
	p.IPv4 = netip.MustParseAddr("10.0.0.2")
	if err := s.UpdatePod(p); err != nil {
		t.Fatal(err)
	}
	if len(w.deleted) != 1 || w.written["a"].IPv4 != p.IPv4 {
		t.Fatal("old IP mapping was not replaced")
	}
}

func TestPodRecreationAndDeleteRetry(t *testing.T) {
	s, w := testSyncer(t)
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "old"}); err != nil {
		t.Fatal(err)
	}
	oldInode := w.written["a"].CgroupInfo.Inode
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "new"}); err != nil {
		t.Fatal(err)
	}
	if w.written["a"].CgroupInfo.Inode == oldInode || len(w.deleted) != 1 {
		t.Fatal("old Pod state reused")
	}
	w.err = errors.New("temporary map failure")
	if err := s.DeletePod("a"); err == nil {
		t.Fatal("expected failure")
	}
	if s.podCache.ByPodID("a") == nil {
		t.Fatal("failed delete discarded retry state")
	}
	w.err = nil
	if err := s.DeletePod("a"); err != nil {
		t.Fatal(err)
	}
	if s.podCache.ByPodID("a") != nil {
		t.Fatal("deleted Pod still cached")
	}
}

func TestPriorityRemovalAndFailedUpdate(t *testing.T) {
	s, w := testSyncer(t)
	prio := uint32(2)
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", HostNetwork: true, Prio: &prio}); err != nil {
		t.Fatal(err)
	}
	if s.cgroup.(*fakeCgroup).classID != 2 {
		t.Fatal("priority not written")
	}
	w.err = errors.New("temporary map failure")
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", HostNetwork: true}); err == nil {
		t.Fatal("expected failure")
	}
	if s.podCache.ByPodID("a").Prio == nil {
		t.Fatal("failed write changed cached input")
	}
	w.err = nil
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", HostNetwork: true}); err != nil {
		t.Fatal(err)
	}
	if s.cgroup.(*fakeCgroup).classID != 0 {
		t.Fatal("priority not restored")
	}
}

func TestPruneRestartedPodState(t *testing.T) {
	s, w := testSyncer(t)
	cg := s.cgroup.(*fakeCgroup)
	cg.groups = map[string]types.CgroupInfo{
		"live":    {Inode: 100}, // Live Pod not yet reconciled after restart.
		"deleted": {Inode: 200}, // Leftover cgroup for a deleted Pod.
	}
	w.rates = map[uint64]struct{}{100: {}, 200: {}, 300: {}}
	// File sync runs before the informer starts and must not perform GC.
	if err := s.syncFilePods(); err != nil {
		t.Fatal(err)
	}
	if w.pruneCalls != 0 {
		t.Fatal("cleaned state before obtaining an authoritative Pod snapshot")
	}
	if err := s.PrunePods(map[string]struct{}{"live": {}}); err != nil {
		t.Fatal(err)
	}
	if len(w.rates) != 1 {
		t.Fatalf("orphaned inodes remain: %v", w.rates)
	}
	if _, ok := w.rates[100]; !ok {
		t.Fatal("deleted limit for an unreconciled live Pod")
	}
	if err := s.PrunePods(map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if len(w.rates) != 0 {
		t.Fatal("empty node snapshot did not clean orphaned limits")
	}
}

func TestPrunePreservesAnnotationAndConcurrentPod(t *testing.T) {
	s, w := testSyncer(t)
	rate := uint64(100)
	if err := s.UpdatePod(&types.PodConfig{PodID: "a", PodUID: "a", TxBps: &rate}); err != nil {
		t.Fatal(err)
	}
	inode := w.written["a"].CgroupInfo.Inode
	w.rates = map[uint64]struct{}{inode: {}, 200: {}}
	// This Pod was reconciled after the API snapshot was taken.
	if err := s.PrunePods(map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.rates[inode]; !ok {
		t.Fatal("removed a newly reconciled Pod's limits")
	}
	checkRate(t, w.written["a"], 0, 100)
}

func TestPruneFailsClosedAndRetries(t *testing.T) {
	s, w := testSyncer(t)
	cg := s.cgroup.(*fakeCgroup)
	w.rates = map[uint64]struct{}{200: {}}
	cg.listErr = errors.New("cgroup scan failed")
	if err := s.PrunePods(nil); err == nil {
		t.Fatal("ignored scan error")
	}
	if w.pruneCalls != 0 || len(w.rates) != 1 {
		t.Fatal("pruned from an incomplete snapshot")
	}
	cg.listErr = nil
	w.pruneErr = errors.New("map deletion failed")
	if err := s.PrunePods(nil); err == nil {
		t.Fatal("ignored map error")
	}
	w.pruneErr = nil
	if err := s.PrunePods(nil); err != nil {
		t.Fatal(err)
	}
	if len(w.rates) != 0 {
		t.Fatal("failed deletion was not retried")
	}
}
