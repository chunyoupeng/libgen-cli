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
	"log"
	"os"
	"regexp"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/chunyoupeng/libgen-cli/libgen"
)

var downloadCmd = &cobra.Command{
	Use:     "download",
	Short:   "Download a specific resource by hash.",
	Long:    `Use this command if you already know the hash of the specific resource you'd like to download.`,
	Example: "libgen download 2F2DBA2A621B693BB95601C16ED680F8",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			if err := cmd.Help(); err != nil {
				fmt.Printf("error displaying CLI help: %v\n", err)
			}
			os.Exit(1)
		}

		// Ensure all provided entries are valid 32-hex MD5 hashes
		md5Regex := regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
		for _, a := range args {
			if !md5Regex.MatchString(a) {
				fmt.Printf("Please provide a valid 32-character MD5 hash: %s\n", a)
				os.Exit(1)
			}
		}

		output, err := cmd.Flags().GetString("output")
		if err != nil {
			fmt.Printf("error getting output flag: %v\n", err)
		}
		useIpfs, err := cmd.Flags().GetBool("ipfs-mirrors")
		if err != nil {
			fmt.Printf("error getting ipfs-mirrors flag: %v\n", err)
		}
		mirror, err := cmd.Flags().GetString("mirror")
		if err != nil {
			fmt.Printf("error getting mirror flag: %v\n", err)
		}

		if len(args) == 1 {
			fmt.Printf("++ Searching for: %s\n", args[0])
		} else {
			fmt.Printf("++ Searching for: MD5s\n")
		}

		searchMirror, pinnedMirror, err := resolveSearchMirror(mirror)
		if err != nil {
			fmt.Printf("error selecting search mirror: %v\n", err)
			os.Exit(1)
		}

		bookDetails, err := libgen.GetDetails(&libgen.GetDetailsOptions{
			Hashes:       args,
			SearchMirror: searchMirror,
			Print:        true,
		})
		if err != nil {
			if pinnedMirror != nil {
				log.Fatalf("error retrieving results from LibGen API: %v", err)
			}
			// Try finite alternative search mirrors without looping infinitely
			for _, m := range libgen.SearchMirrors {
				if m.Host == searchMirror.Host {
					continue
				}
				bookDetails, err = libgen.GetDetails(&libgen.GetDetailsOptions{
					Hashes:       args,
					SearchMirror: m,
					Print:        true,
				})
				if err == nil && len(bookDetails) > 0 {
					break
				}
			}
			if err != nil {
				log.Fatalf("error retrieving results from LibGen API: %v", err)
			}
		}

		if len(bookDetails) == 0 {
			fmt.Println("No book details found for provided hash(es)")
			os.Exit(1)
		}

		for _, book := range bookDetails {
			fmt.Println(strings.Repeat("-", 80))
			fmt.Printf("Download started for: %s by %s\n", book.Title, book.Author)

			if err := libgen.GetDownloadURL(book, useIpfs, pinnedMirror); err != nil {
				fmt.Printf("error getting download URL: %v\n", err)
				os.Exit(1)
			}
			if useIpfs {
				if err := libgen.DownloadBookIPFS(book, output); err != nil {
					fmt.Printf("error downloading %v: %v\n", book.Title, err)
					os.Exit(1)
				}
			} else {
				if err := libgen.DownloadBook(book, output); err != nil {
					fmt.Printf("error downloading %v: %v\n", book.Title, err)
					os.Exit(1)
				}
			}

			if runtime.GOOS == "windows" {
				_, err = fmt.Fprintf(color.Output, "%s %s by %s.%s\n", color.GreenString("[OK]"),
					book.Title, book.Author, book.Extension)
				if err != nil {
					fmt.Printf("error writing to Windows os.Stdout: %v\n", err)
					os.Exit(1)
				}
			} else {
				fmt.Printf("%s %s by %s.%s\n", color.GreenString("[OK]"),
					book.Title, book.Author, book.Extension)
			}
		}
	},
}

func init() {
	downloadCmd.Flags().StringP("output", "o", "", "where you want "+
		"libgen-cli to save your download.")
	downloadCmd.Flags().BoolP("ipfs-mirrors", "i", false, "enforces libgen-cli to download "+
		"results via IPFS mirrors instead of HTTP(S) mirrors.")
	downloadCmd.Flags().StringP("mirror", "m", "", "pin a specific search mirror "+
		"by host (e.g. libgen.li) instead of auto-selecting one. run 'libgen status -m search' to list mirrors.")
}
