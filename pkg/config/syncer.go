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
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"
)

var _ types.SyncPod = &Syncer{}

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
				err = s.syncPodConfig()
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
		log.Info("update pod", "pod", config.PodID)

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
		log.Info("add new pod", "pod", config.PodID)
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
	pods, err := s.parsePerCgroupConfig()
	if err != nil {
		return err
	}
	return s.podChanged(pods)
}

func (s *Syncer) syncPodConfig() error {
	pods, err := s.parsePodConfig()
	if err != nil {
		return err
	}
	return s.podChanged(pods)
}

func (s *Syncer) podChanged(pods []Pod) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	current := sets.New[uint64]()

	for _, pod := range pods {
		info, err := readCgroupInfo(pod.CgroupDir)
		if err != nil {
			log.Error(err, "error get cgroup info", "path", pod.CgroupDir)
			continue
		}

		config := s.podCache.ByCgroupPath(info.Path)
		if config == nil {
			log.Info("ignore pod, cgroup not found", "cgroup", info.Path)
			continue
		}

		if pod.Prio >= 0 && pod.Prio <= 2 {
			prio := uint32(pod.Prio)
			config.Prio = &prio
			config.CgroupInfo.ClassID = prio
		}
		config.RxBps = pod.QoSConfig.IngressBandwidth
		config.TxBps = pod.QoSConfig.EgressBandwidth

		err = s.podChangeLocked(config)
		if err != nil {
			return err
		}
	}

	// clean up old cgroup rate
	cgroups := s.bpf.ListCgroupRate()
	olds := sets.New[uint64]()
	for key := range cgroups {
		olds.Insert(key.Inode)
	}
	for id := range olds.Difference(current) {
		err := s.bpf.DeleteCgroupRate(id)
		if err != nil {
			log.Error(err, "delete cgruop rate failed", "id", strconv.Itoa(int(id)))
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
	for _, pod := range configs {
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
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		cgroupPath := cgroupPathRe.FindString(line)
		rx := parseConfig("rx_bps", line)
		tx := parseConfig("tx_bps", line)

		configs = append(configs, Pod{
			PodName:      "",
			PodNamespace: "",
			PodUID:       "",
			Prio:         -1,
			CgroupDir:    cgroupPath,
			QoSConfig: QoSConfig{
				IngressBandwidth: rx,
				EgressBandwidth:  tx,
			},
		})
	}
	return configs, nil
}

func (s *Syncer) podChangeLocked(config *types.PodConfig) error {
	log.Info("update pod", "pod", config.PodID, "detail", fmt.Sprintf("%+v", config), "prio", *config.Prio)
	err := s.podCache.Update(config)
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
