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

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"
)

var _ types.SyncPod = &Syncer{}
var _ types.PodGarbageCollector = &Syncer{}

const (
	rootFileConfig  = "/var/lib/terway/qos"
	perCgroupConfig = "per_cgroup_bps_limit"
	globalConfig    = "global_bps_config"
	podConfig       = "pod.json"
)

type Syncer struct {
	globalPath    string
	perCgroupPath string
	podConfigPath string

	bpf    bpf.Interface
	cgroup Interface

	podCache *PodCache
	// File configuration is kept separate from annotation-derived pod state.
	filePods map[string]Pod

	lock sync.Mutex
}

func NewSyncer(bpfWriter bpf.Interface) *Syncer {
	return &Syncer{
		globalPath:    filepath.Join(rootFileConfig, globalConfig),
		perCgroupPath: filepath.Join(rootFileConfig, perCgroupConfig),
		podConfigPath: filepath.Join(rootFileConfig, podConfig),

		bpf:    bpfWriter,
		cgroup: NewCgroup(),

		podCache: NewPodCache(),
	}
}

func (s *Syncer) Start(ctx context.Context) error {
	err := os.MkdirAll(rootFileConfig, 0755)
	if err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	log.Info("watching config change", "path", rootFileConfig)
	err = watcher.Add(rootFileConfig)
	if err != nil {
		_ = watcher.Close()
		return err
	}
	if err := s.syncFilePods(); err != nil {
		log.Error(err, "error loading initial pod configuration")
	}

	go func() {
		tick := time.NewTicker(5 * time.Second)

		defer func() { _ = watcher.Close() }()
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				switch event.Name {
				case rootFileConfig:
					if event.Has(fsnotify.Remove | fsnotify.Rename) {
						log.Info("config file gone, will restart", "event", event.String())
						os.Exit(99)
					}
				case s.globalPath:
					log.Info("cfg change", "event", event.String())

					err = s.syncGlobalConfig()
				case s.perCgroupPath:
					log.Info("cfg change", "event", event.String())

					err = s.syncCgroupRate()
				case s.podConfigPath:
					log.Info("cfg change", "event", event.String())

					err = s.syncPodConfig()
				default:
					continue
				}
				if err != nil {
					log.Error(err, "error sync config")
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Error(err, "file watch err")
			case <-tick.C:
				err = s.syncGlobalConfig()
				if err != nil {
					log.Error(err, "error sync config")
				}
				err = s.syncCgroupRate()
				if err != nil {
					log.Error(err, "error sync config")
				}
			}
		}
	}()

	return nil
}

func (s *Syncer) DeletePod(id string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	podConfig := s.podCache.ByPodID(id)
	if podConfig == nil {
		return nil
	}

	if err := s.bpf.DeletePodInfo(podConfig); err != nil {
		return err
	}
	return s.podCache.DelByPodID(id)
}

func (s *Syncer) PrunePods(liveUIDs map[string]struct{}) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Bypass the cgroup lookup TTL: stale cached inodes cannot establish liveness.
	cgroups, err := s.cgroup.ListCgroups()
	if err != nil {
		return err
	}
	keep := make(map[uint64]struct{})
	for uid, cg := range cgroups {
		if _, live := liveUIDs[uid]; live {
			keep[cg.Inode] = struct{}{}
		}
	}
	// Reconciliation may have added Pods after the API snapshot was listed.
	for _, obj := range s.podCache.List() {
		keep[obj.(*types.PodConfig).CgroupInfo.Inode] = struct{}{}
	}
	return s.bpf.PruneCgroupRates(keep)
}

func (s *Syncer) UpdatePod(config *types.PodConfig) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Do not mutate caller-owned objects or the cached cgroup baseline.
	copyConfig := *config
	config = &copyConfig
	v, ok, err := s.podCache.Get(config)
	if err != nil {
		return err
	}
	if ok && v.(*types.PodConfig).PodUID == config.PodUID {
		log.Info("update pod", "pod", config.PodID)

		prev := v.(*types.PodConfig)

		// keep previous cgroup info
		// take only single source
		config.CgroupInfo = prev.CgroupInfo
		if config.IPv4 != prev.IPv4 || config.IPv6 != prev.IPv6 {
			if err := s.bpf.DeletePodInfo(prev); err != nil {
				return err
			}
		}
	} else {
		// new pod
		log.Info("add new pod", "pod", config.PodID)
		cg, err := s.cgroup.GetCgroupByPodUID(config.PodUID)
		if err != nil {
			return err
		}
		config.CgroupInfo = cg
		if ok {
			if err := s.bpf.DeletePodInfo(v.(*types.PodConfig)); err != nil {
				return err
			}
		}
	}

	if err := s.podChangeLocked(config); err != nil {
		return err
	}
	return s.podCache.Update(config)
}

func (s *Syncer) syncGlobalConfig() error {
	ingress, egress, err := GetGlobalConfig(s.globalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return s.bpf.WriteGlobalConfig(ingress, egress)
}

func (s *Syncer) syncCgroupRate() error {
	return s.syncFilePods()
}

func (s *Syncer) syncPodConfig() error {
	return s.syncFilePods()
}

func (s *Syncer) syncFilePods() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Read both sources before applying either, so a missing file cannot erase
	// another source's limits. Invalid input leaves the last snapshot intact.
	pods, err := s.parsePerCgroupConfig()
	if err != nil {
		return err
	}
	jsonPods, err := s.parsePodConfig()
	if err != nil {
		return err
	}
	files := make(map[string]Pod)
	// pod.json overrides the legacy per-cgroup file for matching paths.
	pods = append(pods, jsonPods...)
	for _, pod := range pods {
		path := filepath.Clean(pod.CgroupDir)
		if !filepath.IsAbs(path) {
			return fmt.Errorf("cgroup path must be absolute: %q", pod.CgroupDir)
		}
		if pod.Prio < -1 || pod.Prio > 2 {
			return fmt.Errorf("invalid pod priority: %d", pod.Prio)
		}
		files[path] = pod
	}
	s.filePods = files
	for _, obj := range s.podCache.List() {
		if err := s.podChangeLocked(obj.(*types.PodConfig)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) parsePodConfig() ([]Pod, error) {
	content, err := os.ReadFile(s.podConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	configs := make(map[string]*Pod)
	err = json.Unmarshal(content, &configs)
	if err != nil {
		return nil, err
	}

	var pods []Pod
	seen := make(map[string]bool)
	for _, pod := range configs {
		if pod == nil {
			return nil, fmt.Errorf("pod config must not be null")
		}
		path := filepath.Clean(pod.CgroupDir)
		if seen[path] {
			return nil, fmt.Errorf("duplicate cgroup path: %q", path)
		}
		seen[path] = true
		pods = append(pods, *pod)
	}

	return pods, nil
}

func (s *Syncer) parsePerCgroupConfig() ([]Pod, error) {
	content, err := os.ReadFile(s.perCgroupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	configs := make([]Pod, 0)

	lines := strings.Split(string(content), "\n")
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(strings.ReplaceAll(line, "=", " "))
		if len(fields)%2 != 1 {
			return nil, fmt.Errorf("invalid cgroup configuration on line %d", lineNumber+1)
		}
		rates := make(map[string]uint64)
		for i := 1; i < len(fields); i += 2 {
			key := fields[i]
			if key != "rx_bps" && key != "tx_bps" {
				return nil, fmt.Errorf("unknown bandwidth key %q on line %d", key, lineNumber+1)
			}
			if _, exists := rates[key]; exists {
				return nil, fmt.Errorf("duplicate bandwidth key %q on line %d", key, lineNumber+1)
			}
			value, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid %s on line %d: %w", key, lineNumber+1, err)
			}
			rates[key] = value
		}

		configs = append(configs, Pod{
			PodName:      "",
			PodNamespace: "",
			PodUID:       "",
			Prio:         -1,
			CgroupDir:    fields[0],
			QoSConfig: QoSConfig{
				IngressBandwidth: rates["rx_bps"],
				EgressBandwidth:  rates["tx_bps"],
			},
		})
	}
	return configs, nil
}

func (s *Syncer) podChangeLocked(config *types.PodConfig) error {
	effective := *config
	cg := *config.CgroupInfo
	effective.CgroupInfo = &cg
	if pod, ok := s.filePods[filepath.Clean(cg.Path)]; ok {
		if effective.Prio == nil && pod.Prio >= 0 && pod.Prio <= 2 {
			prio := uint32(pod.Prio)
			effective.Prio = &prio
		}
		if effective.RxBps == nil {
			effective.RxBps = &pod.QoSConfig.IngressBandwidth
		}
		if effective.TxBps == nil {
			effective.TxBps = &pod.QoSConfig.EgressBandwidth
		}
	}
	if effective.Prio != nil {
		if *effective.Prio > 2 {
			return fmt.Errorf("invalid pod priority: %d", *effective.Prio)
		}
		cg.ClassID = *effective.Prio
	}
	log.Info("update pod", "pod", effective.PodID, "classID", cg.ClassID)
	if effective.HostNetwork {
		if err := s.cgroup.SetCgroupClassID(cg.ClassID, cg.Path); err != nil {
			return err
		}
	}
	return s.bpf.WritePodInfo(&effective)
}
