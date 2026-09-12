// Copyright © 2019 Antoine Chiny <antoine.chiny@inria.fr>
// Copyright © 2019 Ryan Ciehanski <ryan@ciehanski.com>
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
	"log"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/chunyoupeng/libgen-cli/libgen"
)

// resolveSearchMirror picks the search mirror to use. When mirrorHost is
// non-empty it pins that specific mirror (and returns a non-nil pointer so the
// download step uses the same one); otherwise it returns a random working
// mirror and a nil pointer (automatic selection downstream).
func resolveSearchMirror(mirrorHost string) (url.URL, *url.URL, error) {
	if mirrorHost != "" {
		m, err := libgen.MirrorByHost(libgen.SearchMirrors, mirrorHost)
		if err != nil {
			return url.URL{}, nil, err
		}
		return m, &m, nil
	}
	m, err := libgen.FindWorkingMirror(libgen.SearchMirrors)
	return m, nil, err
}

func findAlternativeSearchMirror(exclude url.URL) (url.URL, error) {
	var candidates []url.URL
	for _, m := range libgen.SearchMirrors {
		if m.Host != exclude.Host {
			candidates = append(candidates, m)
		}
	}
	return libgen.FindWorkingMirror(candidates)
}

var rootValidArgs = []string{"dbdumps", "download", "download-all", "link", "search", "status", "version"}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "libgen",
	Short: "A command line interface to access Library Genesis' library.",
	Long: `libgen-cli queries Library Genesis, lists all results of a specific query, 
	and makes them available for download. Simple and easy.`,
	//BashCompletionFunction: bashCompletion,
	ValidArgs: rootValidArgs,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	// Add all subcommands to root cmd
	rootCmd.AddCommand(dbdumpsCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(downloadAllCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(completionCmd)

	if len(os.Args) < 2 {
		if err := rootCmd.Help(); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}
	if os.Args[1] == "-v" || os.Args[1] == "version" || os.Args[1] == "--version" {
		fmt.Printf("libgen-cli %v\n", libgen.Version)
		os.Exit(0)
	}

	// Execute libgen-cli cmd
	if err := rootCmd.Execute(); err != nil {
		return err
	}

	return nil
}
