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
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/util/cache"
)

func Test_parseConfig(t *testing.T) {
	contents := `hw_tx_bps_max 100
hw_rx_bps_max 100
offline_l1_tx_bps_min 10
offline_l1_tx_bps_max 20
offline_l2_tx_bps_min 10
offline_l2_tx_bps_max 30
offline_l1_rx_bps_min 10
offline_l1_rx_bps_max 20
offline_l2_rx_bps_min 10
offline_l2_rx_bps_max 30`

	tests := []struct {
		key  string
		want uint64
	}{
		{
			key:  "hw_tx_bps_max",
			want: 100,
		}, {
			key:  "hw_rx_bps_max",
			want: 100,
		}, {
			key:  "offline_l1_tx_bps_min",
			want: 10,
		}, {
			key:  "offline_l1_tx_bps_max",
			want: 20,
		}, {
			key:  "offline_l2_tx_bps_min",
			want: 10,
		}, {
			key:  "offline_l2_tx_bps_max",
			want: 30,
		},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := parseConfig(tt.key, contents); got != tt.want {
				t.Errorf("parseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListCgroupsBypassesLookupCache(t *testing.T) {
	dir := t.TempDir()
	uid := "12345678-1234-1234-1234-123456789abc"
	path := filepath.Join(dir, "pod"+uid)
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(path, "net_cls.classid"), "0")
	cg := &Cgroup{cgroupPath: dir, workPath: []string{"burstable", ""}, cache: cache.NewLRUExpireCache(10)}
	info, err := cg.GetCgroupByPodUID(uid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	groups, err := cg.ListCgroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Fatalf("stale cgroup inode %d survived fresh scan", info.Inode)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := cg.ListCgroups(); err == nil {
		t.Fatal("missing root treated as empty snapshot")
	}
}

func TestListCgroupsRejectsIncompleteScan(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pod12345678-1234-1234-1234-123456789abc")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(path, "net_cls.classid"), "invalid")
	cg := &Cgroup{cgroupPath: dir, workPath: []string{""}}
	if _, err := cg.ListCgroups(); err == nil {
		t.Fatal("accepted partial cgroup scan")
	}
	if err := os.Remove(filepath.Join(path, "net_cls.classid")); err != nil {
		t.Fatal(err)
	}
	if _, err := cg.ListCgroups(); err == nil {
		t.Fatal("missing classid file treated as a removed cgroup")
	}
}
