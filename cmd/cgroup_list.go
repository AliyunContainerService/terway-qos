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

var cgroupListCmd = &cobra.Command{
	Use: "list",
	Run: func(cmd *cobra.Command, args []string) {
		err := cgroupList()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error read bpf map %v", err)
			os.Exit(1)
		}
	},
}

func cgroupList() error {
	var err error

	writer, err := bpf.NewMap()
	if err != nil {
		return err
	}
	defer writer.Close()

	tableData := pterm.TableData{
		{"inode", "direction", "rate"},
	}
	for k, v := range writer.ListCgroupRate() {
		tableData = append(tableData, []string{fmt.Sprintf("%d", k.Inode), fmt.Sprintf("%d", k.Direction), fmt.Sprintf("%d", v.LimitBps)})
	}

	return pterm.DefaultTable.WithHasHeader().WithData(tableData).Render()
}

func init() {
	cgroupCmd.AddCommand(cgroupListCmd)
}
