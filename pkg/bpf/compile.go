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

package bpf

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

const (
	progRoot = "/var/lib/terway"
	progPath = "/var/lib/terway/qos_tc.o"
	progName = "qos_tc"
)

var standardCFlags = []string{"-O2", "-target", "bpf", "-std=gnu99"}

func Compile(enableEDT bool) error {
	custom := map[string]string{}

	if enableEDT {
		custom["FEAT_EDT"] = "1"
	}

	return compile(progName, custom)
}

func compile(name string, custom map[string]string) error {
	args := make([]string, 0, 16)
	args = append(args, "-g")
	args = append(args, standardCFlags...)
	args = append(args, "-I/var/lib/terway/headers")
	for k, v := range custom {
		args = append(args, fmt.Sprintf("-D%s=%s", k, v))
	}
	args = append(args, "-c")
	args = append(args, filepath.Join("/var/lib/terway/src", fmt.Sprintf("%s.c", name)))
	args = append(args, "-o")
	args = append(args, filepath.Join("/var/lib/terway", fmt.Sprintf("%s.o", name)))

	cmd := exec.Command("clang", args...)
	log.Info("exec", "cmd", cmd.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			log.Info(string(out))
		}
		return err
	}
	if len(out) > 0 {
		log.Info(string(out))
	}
	return nil
}
