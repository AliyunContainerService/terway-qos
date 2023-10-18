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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"k8s.io/apimachinery/pkg/util/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

var log = ctrl.Log.WithName("config")

const defaultCgroupRoot = "/sys/fs/cgroup/net_cls/kubepods.slice"

var walkPath = []string{"kubepods-burstable.slice", "kubepods-besteffort.slice", "kubepods-guaranteed.slice"}
var podUIDRe = regexp.MustCompile("[0-9a-fA-F]{8}([-,_][0-9a-fA-F]{4}){3}[-,_][0-9a-fA-F]{12}")

const defaultTTL = 10 * time.Minute
const maxPodPerNode = 1024

var (
	cgroupPathRe = regexp.MustCompile(`^\S+`)
)

type Interface interface {
	GetCgroupByPodUID(string) (*types.CgroupInfo, error)
	SetCgroupClassID(prio uint32, path string) error
}

type Cgroup struct {
	cgroupPath string

	cache *cache.LRUExpireCache
}

func NewCgroup() *Cgroup {
	return &Cgroup{
		cache:      cache.NewLRUExpireCache(maxPodPerNode),
		cgroupPath: defaultCgroupRoot,
	}
}

func (f *Cgroup) GetCgroupByPodUID(id string) (*types.CgroupInfo, error) {
	v, ok := f.cache.Get(id)
	if !ok {
		// update all cache
		result := getCgroupPath()
		for uid, info := range result {
			f.cache.Add(uid, info, defaultTTL)
		}
		v, ok = f.cache.Get(id)
		if !ok {
			return nil, fmt.Errorf("not found")
		}
	}

	info := v.(types.CgroupInfo)
	return &info, nil
}

func (f *Cgroup) SetCgroupClassID(prio uint32, path string) error {
	return os.WriteFile(filepath.Join(path, "net_cls.classid"), []byte(strconv.Itoa(int(prio))), 0644)
}

func GetGlobalConfig(path string) (*types.GlobalConfig, *types.GlobalConfig, error) {
	c, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	ingress := &types.GlobalConfig{}
	egress := &types.GlobalConfig{}

	egress.HwGuaranteed = parseConfig("hw_tx_bps_max", string(c))

	egress.L0MinBps = parseConfig("online_tx_bps_min", string(c))
	egress.L0MaxBps = parseConfig("online_tx_bps_max", string(c))

	egress.L1MinBps = parseConfig("offline_l1_tx_bps_min", string(c))
	egress.L1MaxBps = parseConfig("offline_l1_tx_bps_max", string(c))
	egress.L2MinBps = parseConfig("offline_l2_tx_bps_min", string(c))
	egress.L2MaxBps = parseConfig("offline_l2_tx_bps_max", string(c))

	ingress.HwGuaranteed = parseConfig("hw_rx_bps_max", string(c))

	ingress.L0MinBps = parseConfig("online_rx_bps_min", string(c))
	ingress.L0MaxBps = parseConfig("online_rx_bps_max", string(c))

	ingress.L1MinBps = parseConfig("offline_l1_rx_bps_min", string(c))
	ingress.L1MaxBps = parseConfig("offline_l1_rx_bps_max", string(c))
	ingress.L2MinBps = parseConfig("offline_l2_rx_bps_min", string(c))
	ingress.L2MaxBps = parseConfig("offline_l2_rx_bps_max", string(c))

	return ingress, egress, nil
}

func parseConfig(key string, content string) uint64 {
	re, err := regexp.Compile(fmt.Sprintf("%s(?:=?|\\s+)(\\d+)", key))
	if err != nil {
		return 0
	}
	group := re.FindStringSubmatch(content)
	if len(group) != 2 {
		return 0
	}
	result, _ := strconv.ParseUint(group[1], 10, 64)
	return result
}

func getCgroupPath() map[string]types.CgroupInfo {
	result := map[string]types.CgroupInfo{}
	for _, p := range walkPath {
		path := filepath.Join(defaultCgroupRoot, p)
		entries, err := os.ReadDir(path)
		if os.IsNotExist(err) {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			uid := podUIDRe.FindString(entry.Name())
			if uid == "" {
				continue
			}
			info, err := readCgroupInfo(filepath.Join(path, entry.Name()))
			if err != nil {
				log.Error(err, "error read cgroup info")
			} else {
				result[strings.ReplaceAll(uid, "_", "-")] = info
			}
		}
	}
	return result
}

func readCgroupInfo(path string) (types.CgroupInfo, error) {
	var stat syscall.Stat_t
	err := syscall.Stat(path, &stat)
	if err != nil {
		return types.CgroupInfo{}, err
	}
	// cgroupv1
	classIDBytes, err := os.ReadFile(filepath.Join(path, "net_cls.classid"))
	if err != nil {
		return types.CgroupInfo{}, fmt.Errorf("error read cgroup id, %w", err)
	}

	classID, err := strconv.ParseUint(strings.TrimSpace(string(classIDBytes)), 10, 32)
	if err != nil {
		return types.CgroupInfo{}, fmt.Errorf("failed parse %s,%w", classIDBytes, err)
	}

	return types.CgroupInfo{
		Path:    path,
		ClassID: uint32(classID),
		Inode:   stat.Ino,
	}, nil
}
