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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"
)

var _ types.SyncPod = &Syncer{}

const (
	rootFileConfig  = "/var/lib/terway/qos"
	perCgroupConfig = "per_cgroup_bps_limit"
	globalConfig    = "global_bps_config"
)

type Syncer struct {
	globalPath    string
	perCgroupPath string

	bpf    bpf.Interface
	cgroup Interface

	podCache *PodCache

	lock sync.Mutex
}

func NewSyncer(bpfWriter bpf.Interface) *Syncer {
	return &Syncer{
		globalPath:    filepath.Join(rootFileConfig, globalConfig),
		perCgroupPath: filepath.Join(rootFileConfig, perCgroupConfig),

		bpf:    bpfWriter,
		cgroup: NewCgroup(),

		podCache: NewPodCache(),
	}
}

func (s *Syncer) Start(ctx context.Context) error {
	err := os.MkdirAll(rootFileConfig, os.ModeDir)
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
		return err
	}

	go func() {
		tick := time.NewTicker(5 * time.Second)

		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				tick.Stop()
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
				case s.globalPath, s.perCgroupPath:
				default:
					continue
				}

				log.Info("cfg change", "event", event.String())

				if event.Name == s.globalPath {
					err = s.syncGlobalConfig()
					if err != nil {
						log.Error(err, "error sync config")
					}
				}
				if event.Name == s.perCgroupPath {
					err = s.syncCgroupRate()
					if err != nil {
						log.Error(err, "error sync config")
					}
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
	if err := s.podCache.DelByPodID(id); err != nil {
		return err
	}

	return s.bpf.DeletePodInfo(podConfig)
}

func (s *Syncer) UpdatePod(config *types.PodConfig) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	prio := config.Prio

	v, ok, err := s.podCache.Get(config)
	if err != nil {
		return err
	}
	if ok {
		prev := v.(*types.PodConfig)

		// keep previous cgroup info
		// take only single source
		config.CgroupInfo = prev.CgroupInfo

		// annotation has higher priority
		if config.TxBps == 0 && prev.TxBps > 0 {
			config.TxBps = prev.TxBps
		}
		if config.RxBps == 0 && prev.RxBps > 0 {
			config.RxBps = prev.RxBps
		}
	} else {
		// new pod
		cg, err := s.cgroup.GetCgroupByPodUID(config.PodUID)
		if err != nil {
			return err
		}
		config.CgroupInfo = cg
	}

	if prio != nil && *prio <= 2 {
		config.CgroupInfo.ClassID = *prio
	}

	err = s.podCache.Update(config)
	if err != nil {
		return err
	}

	if config.HostNetwork && config.Prio != nil {
		err = s.cgroup.SetCgroupClassID(*config.Prio, config.CgroupInfo.Path)
		if err != nil {
			return err
		}
	}

	return s.bpf.WritePodInfo(config)
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
	content, err := os.ReadFile(s.perCgroupPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	lines := strings.Split(string(content), "\n")
	if err != nil {
		return err
	}

	current := sets.New[uint64]()
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		cgroupPath := cgroupPathRe.FindString(line)

		info, err := readCgroupInfo(cgroupPath)
		if err != nil {
			log.Error(err, "error get cgroup info", "path", cgroupPath)
			continue
		}

		rx := parseConfig("rx_bps", line)
		tx := parseConfig("tx_bps", line)

		rate := &types.CgroupRate{
			Inode: info.Inode,
			RxBps: rx,
			TxBps: tx,
		}

		s.lock.Lock()
		podConfig := s.podCache.ByCgroupPath(info.Path)
		if podConfig == nil {
			s.lock.Unlock()
			continue
		}
		podConfig.RxBps = rx
		podConfig.TxBps = tx
		err = s.podCache.Update(podConfig)
		if err != nil {
			s.lock.Unlock()
			return err
		}
		s.lock.Unlock()

		err = s.bpf.WritePodInfo(podConfig)
		if err != nil {
			return err
		}
		current.Insert(rate.Inode)
	}

	olds := s.bpf.GetCgroupRateInodes()
	for id := range olds.Difference(current) {
		err = s.bpf.DeleteCgroupRate(id)
		if err != nil {
			log.Error(err, "delete cgruop rate failed", "id", strconv.Itoa(int(id)))
		}
	}

	// write the info to cgroup
	return nil
}
