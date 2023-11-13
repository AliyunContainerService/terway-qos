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

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"k8s.io/klog/v2"

	"github.com/AliyunContainerService/terway-qos/pkg/bpf"
	"github.com/AliyunContainerService/terway-qos/pkg/config"
	"github.com/AliyunContainerService/terway-qos/pkg/k8s"
	"github.com/AliyunContainerService/terway-qos/pkg/version"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	enableBPFCORE = "enable-bpf-core"
	enableIngress = "enable-ingress"
	enableEgress  = "enable-egress"
)

func init() {
	fs := pflag.NewFlagSet("daemon", pflag.PanicOnError)
	fs.Bool(enableBPFCORE, false, "enable bpf CORE")
	fs.Bool(enableIngress, false, "enable ingress direction qos")
	fs.Bool(enableEgress, false, "enable egress direction qos")

	_ = viper.BindPFlags(fs)
	pflag.CommandLine.AddFlagSet(fs)

	rootCmd.AddCommand(daemonCmd)

	cobra.OnInitialize(initConfig)
}

var daemonCmd = &cobra.Command{
	Use:     "daemon",
	Aliases: []string{"d"},
	Short:   "start daemon",
	Run: func(cmd *cobra.Command, args []string) {
		klog.Infof("version: %s", version.Version)
		err := daemon()
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}
	},
}

func daemon() error {
	ctx := ctrl.SetupSignalHandler()
	ctrl.SetLogger(klogr.New())

	mgr, err := bpf.NewBpfMgr(viper.GetBool(enableIngress), viper.GetBool(enableEgress), viper.GetBool(enableBPFCORE))
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
}

func initConfig() {
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
