// Copyright © 2020 Ryan Ciehanski <ryan@ciehanski.com>
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
	"os"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/chunyoupeng/libgen-cli/libgen"
)

var linkCmd = &cobra.Command{
	Use:     "link",
	Short:   "Retrieves and displays the direct download link for a specific resource.",
	Long:    `Retrieves and displays the direct download link for a specific resource.`,
	Example: "libgen link 2F2DBA2A621B693BB95601C16ED680F8",
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) != 1 {
			if err := cmd.Help(); err != nil {
				fmt.Printf("error displaying CLI help: %v\n", err)
			}
			os.Exit(1)
		}
		// Ensure provided entry is valid MD5 hash
		re := regexp.MustCompile(libgen.SearchMD5)
		if !re.MatchString(args[0]) {
			fmt.Printf("Please provide a valid MD5 hash\n")
			os.Exit(1)
		}

		// Get flags
		useIpfs, err := cmd.Flags().GetBool("ipfs-mirrors")
		if err != nil {
			fmt.Printf("error getting ipfs-mirrors flag: %v\n", err)
		}
		mirror, err := cmd.Flags().GetString("mirror")
		if err != nil {
			fmt.Printf("error getting mirror flag: %v\n", err)
		}

		fmt.Printf("++ Retrieving download link for: %s\n", args[0])

		searchMirror, pinnedMirror, err := resolveSearchMirror(mirror)
		if err != nil {
			fmt.Printf("error selecting search mirror: %v\n", err)
			os.Exit(1)
		}
		bookDetails, err := libgen.GetDetails(&libgen.GetDetailsOptions{
			Hashes:       args,
			SearchMirror: searchMirror,
			Print:        false,
		})
		if err != nil {
			// If error and no mirror was pinned, try another mirror before exiting
			if pinnedMirror != nil {
				log.Fatalf("error retrieving results from LibGen API: %v", err)
			}
			secondaryMirror, altErr := findAlternativeSearchMirror(searchMirror)
			if altErr != nil {
				log.Fatalf("error retrieving results from LibGen API (and no alternative mirror available): %v", err)
			}
			bookDetails, err = libgen.GetDetails(&libgen.GetDetailsOptions{
				Hashes:       args,
				SearchMirror: secondaryMirror,
				Print:        false,
			})
			if err != nil {
				log.Fatalf("error retrieving results from LibGen API: %v", err)
			}
		}
		book := bookDetails[0]

		if err := libgen.GetDownloadURL(book, useIpfs, pinnedMirror); err != nil {
			fmt.Printf("error getting download URL: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%v\n", book.DownloadURL)
	},
}

func init() {
	linkCmd.Flags().BoolP("ipfs-mirrors", "i", false, "enforces libgen-cli to download "+
		"results via IPFS mirrors instead of HTTP(S) mirrors.")
	linkCmd.Flags().StringP("mirror", "m", "", "pin a specific search mirror "+
		"by host (e.g. libgen.li) instead of auto-selecting one. run 'libgen status -m search' to list mirrors.")
}
