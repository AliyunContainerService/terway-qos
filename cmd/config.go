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
	"time"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/types"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var direction string
var watch bool

var (
	cgroupPath string
	ipv4       string
	ipv6       string
	rate       uint64 // bytes/s
	priority   int
)

var (
	hwRxGuaranteedRate uint64
	hwTxGuaranteedRate uint64

	adjustInterval uint64

	l1RxMaxRate uint64
	l1RxMinRate uint64

	l1TxMaxRate uint64
	l1TxMinRate uint64

	l2RxMaxRate uint64
	l2RxMinRate uint64

	l2TxMaxRate uint64
	l2TxMinRate uint64
)

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "config qos",
}

var globalCmd = &cobra.Command{
	Use:   "global",
	Short: "g",
}

var globalSetCmd = &cobra.Command{
	Use: "set",
	RunE: func(cmd *cobra.Command, args []string) error {
		writer, err := bpf.NewMap()
		if err != nil {
			return err
		}
		defer writer.Close()

		egress := &types.GlobalConfig{
			HwGuaranteed:   hwTxGuaranteedRate,
			HwBurstableBps: hwTxGuaranteedRate,
			L1MaxBps:       l1TxMaxRate,
			L1MinBps:       l1TxMinRate,
			L2MaxBps:       l2TxMaxRate,
			L2MinBps:       l2TxMinRate,
		}
		ingress := &types.GlobalConfig{
			HwGuaranteed:   hwRxGuaranteedRate,
			HwBurstableBps: hwRxGuaranteedRate,
			L1MaxBps:       l1RxMaxRate,
			L1MinBps:       l1RxMinRate,
			L2MaxBps:       l2RxMaxRate,
			L2MinBps:       l2RxMinRate,
		}

		err = writer.WriteGlobalConfig(ingress, egress)
		if err != nil {
			return err
		}

		return nil
	},
}

var globalGetCmd = &cobra.Command{
	Use: "get",
	RunE: func(cmd *cobra.Command, args []string) error {
		writer, err := bpf.NewMap()
		if err != nil {
			return err
		}
		defer writer.Close()
		ing, eg, err := writer.GetGlobalConfig()
		if err != nil {
			return err
		}

		return pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
			{"", "L0", "L1", "L2"},
			{"Rx-Max", fmt.Sprintf("%d", ing.HwGuaranteed), fmt.Sprintf("%d", ing.L1MaxBps), fmt.Sprintf("%d", ing.L2MaxBps)},
			{"Rx-Min", fmt.Sprintf("%d", ing.L0MinBps), fmt.Sprintf("%d", ing.L1MinBps), fmt.Sprintf("%d", ing.L2MinBps)},
			{"Tx-Max", fmt.Sprintf("%d", eg.HwGuaranteed), fmt.Sprintf("%d", eg.L1MaxBps), fmt.Sprintf("%d", eg.L2MaxBps)},
			{"Tx-Min", fmt.Sprintf("%d", eg.L0MinBps), fmt.Sprintf("%d", eg.L1MinBps), fmt.Sprintf("%d", eg.L2MinBps)},
		}).Render()
	},
}

var globalRateCetCmd = &cobra.Command{
	Use: "rate",
	RunE: func(cmd *cobra.Command, args []string) error {
		writer, err := bpf.NewMap()
		if err != nil {
			return err
		}
		defer writer.Close()
		_, eg := writer.GetGlobalRateLimit()
		if err != nil {
			return err
		}

		return pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
			{"", "L0", "L1", "L2"},
			{"Tx-Max", fmt.Sprintf("%d", eg.L0Bps), fmt.Sprintf("%d", eg.L1Bps), fmt.Sprintf("%d", eg.L2Bps)},
			{"last", fmt.Sprintf("%d", eg.L0LastTimestamp), fmt.Sprintf("%d", eg.L1LastTimestamp), fmt.Sprintf("%d", eg.L2LastTimestamp)},
			{"start", fmt.Sprintf("%d", eg.LastTimestamp), fmt.Sprintf("%d", eg.LastTimestamp), fmt.Sprintf("%d", eg.LastTimestamp)},
		}).Render()
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(globalRateCetCmd)
	configCmd.AddCommand(podCmd, globalCmd)

	globalCmd.AddCommand(globalSetCmd, globalGetCmd)
	globalSetCmd.PersistentFlags().Uint64Var(&adjustInterval, "interval", uint64(1*time.Second), "interval to adjust bandwidth")
	globalSetCmd.PersistentFlags().Uint64Var(&hwRxGuaranteedRate, "hw-rx", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&hwTxGuaranteedRate, "hw-tx", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l1TxMaxRate, "l1-tx-max", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l1TxMinRate, "l1-tx-min", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l2TxMaxRate, "l2-tx-max", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l2TxMinRate, "l2-tx-min", 0, "")

	globalSetCmd.PersistentFlags().Uint64Var(&l1RxMaxRate, "l1-rx-max", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l1RxMinRate, "l1-rx-min", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l2RxMaxRate, "l2-rx-max", 0, "")
	globalSetCmd.PersistentFlags().Uint64Var(&l2RxMinRate, "l2-rx-min", 0, "")

	_ = globalSetCmd.MarkPersistentFlagRequired("hw-rx")
	_ = globalSetCmd.MarkPersistentFlagRequired("hw-tx")
	_ = globalSetCmd.MarkPersistentFlagRequired("l1-tx-max")
	_ = globalSetCmd.MarkPersistentFlagRequired("l1-tx-min")
	_ = globalSetCmd.MarkPersistentFlagRequired("l2-tx-max")
	_ = globalSetCmd.MarkPersistentFlagRequired("l2-tx-min")
	_ = globalSetCmd.MarkPersistentFlagRequired("l1-rx-max")
	_ = globalSetCmd.MarkPersistentFlagRequired("l1-rx-min")
	_ = globalSetCmd.MarkPersistentFlagRequired("l2-rx-max")
	_ = globalSetCmd.MarkPersistentFlagRequired("l2-rx-min")

	globalGetCmd.PersistentFlags().BoolVar(&watch, "w", false, "watch")
}
