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
	"os"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var bpfBandwidthListCmd = &cobra.Command{
	Use: "list",
	Run: func(cmd *cobra.Command, args []string) {
		writer, err := bpf.NewMap()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error init bpf map %v", err)
			os.Exit(1)
		}
		defer writer.Close()
		ing, eg, err := writer.GetGlobalConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error get global config %v", err)
			os.Exit(1)
		}
		err = pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
			{"config", "l0", "l1", "l2"},
			{"rx-max", fmt.Sprintf("%d", ing.HwGuaranteed), fmt.Sprintf("%d", ing.L1MaxBps), fmt.Sprintf("%d", ing.L2MaxBps)},
			{"rx-min", fmt.Sprintf("%d", ing.L0MinBps), fmt.Sprintf("%d", ing.L1MinBps), fmt.Sprintf("%d", ing.L2MinBps)},
			{"tx-max", fmt.Sprintf("%d", eg.HwGuaranteed), fmt.Sprintf("%d", eg.L1MaxBps), fmt.Sprintf("%d", eg.L2MaxBps)},
			{"tx-min", fmt.Sprintf("%d", eg.L0MinBps), fmt.Sprintf("%d", eg.L1MinBps), fmt.Sprintf("%d", eg.L2MinBps)},
		}).Render()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error get global config %v", err)
			os.Exit(1)
		}

		ingRate, egressRate := writer.GetGlobalRateLimit()
		_ = pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
			{"limit", "L0", "L1", "L2"},
			{"tx-max", fmt.Sprintf("%d", egressRate.L0Bps), fmt.Sprintf("%d", egressRate.L1Bps), fmt.Sprintf("%d", egressRate.L2Bps)},
			{"t_last", fmt.Sprintf("%d", egressRate.L0LastTimestamp), fmt.Sprintf("%d", egressRate.L1LastTimestamp), fmt.Sprintf("%d", egressRate.L2LastTimestamp)},
			{"slot", fmt.Sprintf("%d", egressRate.L0Slot), fmt.Sprintf("%d", egressRate.L1Slot), fmt.Sprintf("%d", egressRate.L2Slot)},
		}).Render()

		_ = pterm.DefaultTable.WithHasHeader().WithData(pterm.TableData{
			{"limit", "L0", "L1", "L2"},
			{"rx-max", fmt.Sprintf("%d", ingRate.L0Bps), fmt.Sprintf("%d", ingRate.L1Bps), fmt.Sprintf("%d", ingRate.L2Bps)},
			{"t_last", fmt.Sprintf("%d", ingRate.L0LastTimestamp), fmt.Sprintf("%d", ingRate.L1LastTimestamp), fmt.Sprintf("%d", ingRate.L2LastTimestamp)},
			{"slot", fmt.Sprintf("%d", ingRate.L0Slot), fmt.Sprintf("%d", ingRate.L1Slot), fmt.Sprintf("%d", ingRate.L2Slot)},
		}).Render()

		data := [][]string{
			{"stat", "index", "ts", "val"},
		}
		for _, v := range writer.GetNetStat() {
			data = append(data, []string{"", fmt.Sprintf("%d", v.Index), fmt.Sprintf("%d", v.TS), fmt.Sprintf("%d", v.Val)})
		}
		_ = pterm.DefaultTable.WithHasHeader().WithData(data).Render()

	},
}

func init() {
	bpfBandwidthCmd.AddCommand(bpfBandwidthListCmd)
}
