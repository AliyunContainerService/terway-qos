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

package cmd

import (
	"fmt"
	"net/netip"
	"os"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"github.com/spf13/cobra"
)

var podSetCmd = &cobra.Command{
	Use: "set",
	Run: func(cmd *cobra.Command, args []string) {
		err := podSet()
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func podSet() error {
	if ipv4 == "" && ipv6 == "" {
		return fmt.Errorf("ip must provided")
	}
	var err error
	var v4, v6 netip.Addr
	if ipv4 != "" {
		v4, err = netip.ParseAddr(ipv4)
		if err != nil {
			return err
		}
	}
	if ipv6 != "" {
		v6, err = netip.ParseAddr(ipv6)
		if err != nil {
			return err
		}
	}
	writer, err := bpf.NewMap()
	if err != nil {
		return err
	}
	defer writer.Close()

	unSet := uint64(0)
	return writer.WritePodInfo(&types.PodConfig{
		PodID:       "",
		PodUID:      "",
		IPv4:        v4,
		IPv6:        v6,
		HostNetwork: false,
		CgroupInfo:  nil,
		RxBps:       &unSet,
		TxBps:       &rate,
	})
}

func init() {
	podCmd.AddCommand(podSetCmd)

	podCmd.PersistentFlags().StringVar(&direction, "direction", "egress", "ingress or egress")
	podCmd.PersistentFlags().StringVar(&cgroupPath, "cgroup", "", "cgroup path.")
	podCmd.PersistentFlags().StringVar(&ipv4, "ipv4", "", "ipv4 addr")
	podCmd.PersistentFlags().StringVar(&ipv6, "ipv6", "", "ipv6 addr")
	podCmd.PersistentFlags().Uint64Var(&rate, "rate", 0, "rate limit. bytes/s. At lease 1 MB/s, set 0 to disable rate limit")
	podCmd.PersistentFlags().IntVar(&priority, "prio", 0, "priority. 0,1,2")

	_ = podSetCmd.MarkPersistentFlagRequired("cgroup")
	_ = podSetCmd.MarkPersistentFlagRequired("rate")
}
