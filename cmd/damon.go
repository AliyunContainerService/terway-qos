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
	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/config"
	"github.com/AliyunContainerService/terway-qos/pkg/k8s"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
)

var daemonCmd = &cobra.Command{
	Use:     "daemon",
	Aliases: []string{"d"},
	Short:   "start daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := ctrl.SetupSignalHandler()
		ctrl.SetLogger(klogr.New())

		mgr, err := bpf.NewBpfMgr()
		if err != nil {
			return err
		}
		err = mgr.Start(ctx)
		if err != nil {
			return err
		}
		m, err := bpf.NewMap()
		if err != nil {
			return err
		}
		defer m.Close()

		syncer := config.NewSyncer(m)
		err = syncer.Start(ctx)
		if err != nil {
			return err
		}
		return k8s.StartPodHandler(ctx, syncer)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
