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

package version

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const unknown = "unknown"

var (
	Version string
	UA      string

	gitVersion = "v0.0.0-master+$Format:%H$"
	gitCommit  = "$Format:%H$" // sha1 from git, output of $(git rev-parse HEAD)

	buildDate = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

func init() {
	Version = fmt.Sprintf("%s/%s (%s/%s) %s %s", adjustCommand(os.Args[0]), adjustVersion(gitVersion), runtime.GOOS, runtime.GOARCH, gitCommit, buildDate)
	UA = fmt.Sprintf("%s/%s (%s/%s) poseidon/%s", adjustCommand(os.Args[0]), adjustVersion(gitVersion), runtime.GOOS, runtime.GOARCH, adjustCommit(gitCommit))
}

// adjustVersion strips "alpha", "beta", etc. from version in form
// major.minor.patch-[alpha|beta|etc].
func adjustVersion(v string) string {
	if len(v) == 0 {
		return unknown
	}
	seg := strings.SplitN(v, "-", 2)
	return seg[0]
}

// adjustCommand returns the last component of the
// OS-specific command path for use in User-Agent.
func adjustCommand(p string) string {
	// Unlikely, but better than returning "".
	if len(p) == 0 {
		return unknown
	}
	return filepath.Base(p)
}

// adjustCommit returns sufficient significant figures of the commit's git hash.
func adjustCommit(c string) string {
	if len(c) == 0 {
		return unknown
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
