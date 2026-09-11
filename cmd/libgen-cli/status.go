// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
// Copyright © 2026 Chunyou Peng <chunyoupeng@gmail.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package libgen_cli

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sync"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/chunyoupeng/libgen-cli/libgen"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Checks the status of Library Genesis' mirrors.",
	Long:    `Checks the status of all Library Genesis search mirrors as well as all download mirrors concurrently.`,
	Example: `libgen status`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 0 {
			if err := cmd.Help(); err != nil {
				fmt.Printf("error displaying CLI help: %v\n", err)
			}
			os.Exit(0)
		}

		mirror, err := cmd.Flags().GetString("mirror")
		if err != nil {
			fmt.Printf("error getting mirror flag: %v\n", err)
		}

		switch mirror {
		case "download":
			probeMirrorsConcurrently(libgen.DownloadMirrors)
		case "search":
			probeMirrorsConcurrently(libgen.SearchMirrors)
		default:
			probeMirrorsConcurrently(append(libgen.SearchMirrors, libgen.DownloadMirrors...))
		}
	},
}

func probeMirrorsConcurrently(mirrors []url.URL) {
	type probeResult struct {
		host   string
		status int
	}

	results := make([]probeResult, len(mirrors))
	var wg sync.WaitGroup

	for i, m := range mirrors {
		wg.Add(1)
		go func(idx int, target url.URL) {
			defer wg.Done()
			status := libgen.CheckMirror(target)
			results[idx] = probeResult{
				host:   target.Host,
				status: status,
			}
		}(i, m)
	}
	wg.Wait()

	for _, res := range results {
		if res.status == http.StatusOK {
			if runtime.GOOS == "windows" {
				_, _ = fmt.Fprintf(color.Output, "%s %s\n", color.GreenString("[OK]"), res.host)
			} else {
				fmt.Printf("%s %s\n", color.GreenString("[OK]"), res.host)
			}
		} else {
			if runtime.GOOS == "windows" {
				_, _ = fmt.Fprintf(color.Output, "%s %s\n", color.RedString("[FAIL]"), res.host)
			} else {
				fmt.Printf("%s %s\n", color.RedString("[FAIL]"), res.host)
			}
		}
	}
}

func init() {
	statusCmd.Flags().StringP("mirror", "m", "", "checks the status of "+
		"the specified mirrors. (search, download)")
}
